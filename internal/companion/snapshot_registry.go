package companion

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/channing771/mornlea/internal/pathfind"
)

const (
	// SnapshotRegistryCapacity 是同时存活的冻结规划快照硬上限。
	SnapshotRegistryCapacity = 4
	// snapshotExpiryGrace 为 run deadline 后留给已在途 bounded read 的固定宽限。
	snapshotExpiryGrace = 5 * time.Second
)

var (
	// ErrSnapshotRegistryFull 表示四槽已满；调用方必须立即失败而不是等待。
	ErrSnapshotRegistryFull = errors.New("companion: snapshot registry 已满")
	// ErrSnapshotRegistryClosed 表示 registry 已关闭且不再接受注册。
	ErrSnapshotRegistryClosed = errors.New("companion: snapshot registry 已关闭")
	// ErrSnapshotUnavailable 统一表示 capability 无效、过期、取消或已完成。
	ErrSnapshotUnavailable = errors.New("companion: snapshot 不可用")
	// ErrSnapshotInvalid 表示注册身份、deadline 或快照不满足契约。
	ErrSnapshotInvalid = errors.New("companion: snapshot 注册参数非法")
)

type snapshotTimer interface {
	Stop() bool
}

type snapshotClock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) snapshotTimer
}

type realSnapshotClock struct{}

func (realSnapshotClock) Now() time.Time { return time.Now() }
func (realSnapshotClock) AfterFunc(delay time.Duration, fn func()) snapshotTimer {
	return time.AfterFunc(delay, fn)
}

// SnapshotRegistration 是注册成功后交给 Agent HTTP 请求的运行身份。
type SnapshotRegistration struct {
	SnapshotID string
	Capability string
	Digest     string
	Deadline   time.Time
	ExpiresAt  time.Time
}

// SnapshotLease 是一次 MCP handler 取得的不可变深拷贝视图。Done 只由 registry
// 生命周期关闭；handler 必须在入口、bounded loop 与编码边界调用 Checkpoint。
type SnapshotLease struct {
	SnapshotID  string
	NamespaceID string
	CompanionID ID
	Generation  uint64
	Digest      string
	Snapshot    PlanSnapshot
	Deadline    time.Time
	ExpiresAt   time.Time
	done        <-chan struct{}
}

// Done 返回 registry-owned cancellation signal。
func (l SnapshotLease) Done() <-chan struct{} { return l.done }

// Checkpoint 在租约已完成、取消、过期或关闭时返回统一不可用错误。
func (l SnapshotLease) Checkpoint() error {
	select {
	case <-l.done:
		return ErrSnapshotUnavailable
	default:
		return nil
	}
}

type snapshotRecord struct {
	id          string
	capability  string
	namespaceID string
	companionID ID
	generation  uint64
	digest      string
	snapshot    PlanSnapshot
	deadline    time.Time
	expiresAt   time.Time
	context     context.Context
	cancel      context.CancelFunc
	timer       snapshotTimer
}

// SnapshotRegistry 持有至多四份有界冻结快照。锁内只维护 map、时间与生命周期
// 状态；digest 编码、深拷贝和随机身份生成均在锁外完成。
type SnapshotRegistry struct {
	mu           sync.Mutex
	clock        snapshotClock
	entropy      io.Reader
	closed       bool
	byCapability map[string]*snapshotRecord
	byID         map[string]*snapshotRecord
}

// NewSnapshotRegistry 创建使用系统时钟与 crypto/rand 的生产 registry。
func NewSnapshotRegistry() *SnapshotRegistry {
	return newSnapshotRegistry(realSnapshotClock{}, rand.Reader)
}

func newSnapshotRegistry(clock snapshotClock, entropy io.Reader) *SnapshotRegistry {
	if clock == nil {
		clock = realSnapshotClock{}
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	return &SnapshotRegistry{
		clock:        clock,
		entropy:      entropy,
		byCapability: make(map[string]*snapshotRecord, SnapshotRegistryCapacity),
		byID:         make(map[string]*snapshotRecord, SnapshotRegistryCapacity),
	}
}

// Register 深拷贝并注册一份快照。容量检查不等待；deadline 必须严格晚于当前
// wall clock，expiry 固定为 deadline+5s 且不得溢出。
func (r *SnapshotRegistry) Register(namespaceID string, companionID ID, generation uint64, snapshot PlanSnapshot, deadline time.Time) (SnapshotRegistration, error) {
	now := r.clock.Now()
	if !validCanonicalAgentID(namespaceID) || !companionID.Valid() || generation == 0 || !deadline.After(now) {
		return SnapshotRegistration{}, ErrSnapshotInvalid
	}
	expiresAt := deadline.Add(snapshotExpiryGrace)
	if !expiresAt.After(deadline) || expiresAt.Year() > 9999 || !snapshotTimerDelayRepresentable(now, deadline, expiresAt) {
		return SnapshotRegistration{}, ErrSnapshotInvalid
	}
	r.reapExpired(now)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return SnapshotRegistration{}, ErrSnapshotRegistryClosed
	}
	if len(r.byID) >= SnapshotRegistryCapacity {
		r.mu.Unlock()
		return SnapshotRegistration{}, ErrSnapshotRegistryFull
	}
	r.mu.Unlock()

	_, digest, err := CanonicalSnapshotDigest(snapshot)
	if err != nil {
		return SnapshotRegistration{}, fmt.Errorf("%w: %v", ErrSnapshotInvalid, err)
	}
	frozen := clonePlanSnapshot(snapshot)
	id, capability, err := r.newIdentity()
	if err != nil {
		return SnapshotRegistration{}, fmt.Errorf("%w: 生成快照身份: %v", ErrSnapshotInvalid, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	record := &snapshotRecord{
		id: id, capability: capability, namespaceID: namespaceID, companionID: companionID,
		generation: generation, digest: digest, snapshot: frozen, deadline: deadline,
		expiresAt: expiresAt, context: ctx, cancel: cancel,
	}

	finalNow := r.clock.Now()
	if !snapshotTimerDelayRepresentable(finalNow, deadline, expiresAt) {
		cancel()
		return SnapshotRegistration{}, ErrSnapshotInvalid
	}
	r.reapExpired(finalNow)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return SnapshotRegistration{}, ErrSnapshotRegistryClosed
	}
	if len(r.byID) >= SnapshotRegistryCapacity {
		r.mu.Unlock()
		cancel()
		return SnapshotRegistration{}, ErrSnapshotRegistryFull
	}
	if _, exists := r.byID[id]; exists {
		r.mu.Unlock()
		cancel()
		return SnapshotRegistration{}, fmt.Errorf("%w: snapshot ID 冲突", ErrSnapshotInvalid)
	}
	if _, exists := r.byCapability[capability]; exists {
		r.mu.Unlock()
		cancel()
		return SnapshotRegistration{}, fmt.Errorf("%w: capability 冲突", ErrSnapshotInvalid)
	}
	scheduleNow := r.clock.Now()
	if !snapshotTimerDelayRepresentable(scheduleNow, deadline, expiresAt) {
		r.mu.Unlock()
		cancel()
		return SnapshotRegistration{}, ErrSnapshotInvalid
	}
	r.byID[id] = record
	r.byCapability[capability] = record
	delay := expiresAt.Sub(scheduleNow)
	record.timer = r.clock.AfterFunc(delay, func() { r.expire(id) })
	r.mu.Unlock()
	return SnapshotRegistration{
		SnapshotID: id, Capability: capability, Digest: digest,
		Deadline: deadline, ExpiresAt: expiresAt,
	}, nil
}

func snapshotTimerDelayRepresentable(now, deadline, expiresAt time.Time) bool {
	if !deadline.After(now) || !expiresAt.After(deadline) {
		return false
	}
	delay := expiresAt.Sub(now)
	return delay > 0 && now.Add(delay).Equal(expiresAt)
}

// Lookup 只按 capability 定位 record，并返回与 registry 存储分离的深拷贝。
// 所有无效身份统一返回 ErrSnapshotUnavailable，避免泄露 existence。
func (r *SnapshotRegistry) Lookup(capability string) (SnapshotLease, error) {
	now := r.clock.Now()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return SnapshotLease{}, ErrSnapshotUnavailable
	}
	record, ok := r.byCapability[capability]
	if !ok || subtle.ConstantTimeCompare([]byte(record.capability), []byte(capability)) != 1 {
		r.mu.Unlock()
		return SnapshotLease{}, ErrSnapshotUnavailable
	}
	if !now.Before(record.expiresAt) {
		r.removeLocked(record)
		r.mu.Unlock()
		record.cancel()
		return SnapshotLease{}, ErrSnapshotUnavailable
	}
	lease := SnapshotLease{
		SnapshotID: record.id, NamespaceID: record.namespaceID, CompanionID: record.companionID,
		Generation: record.generation, Digest: record.digest, Deadline: record.deadline,
		ExpiresAt: record.expiresAt, done: record.context.Done(),
	}
	r.mu.Unlock()
	if lease.Checkpoint() != nil {
		return SnapshotLease{}, ErrSnapshotUnavailable
	}
	// record 的 snapshot 在插入后永不改写；删除只移除 map 引用并关闭 done。
	// 因而大数组与 slice 深拷贝可安全留在 registry mutex 外。
	lease.Snapshot = clonePlanSnapshot(record.snapshot)
	if lease.Checkpoint() != nil {
		return SnapshotLease{}, ErrSnapshotUnavailable
	}
	return lease, nil
}

// Complete 删除正常完成的快照并发出取消信号。
func (r *SnapshotRegistry) Complete(snapshotID string) bool { return r.finish(snapshotID) }

// Cancel 删除显式取消的快照并发出取消信号。
func (r *SnapshotRegistry) Cancel(snapshotID string) bool { return r.finish(snapshotID) }

func (r *SnapshotRegistry) finish(snapshotID string) bool {
	r.mu.Lock()
	record, ok := r.byID[snapshotID]
	if ok {
		r.removeLocked(record)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	record.cancel()
	return true
}

// Close 幂等删除全部 record、停止 timer、发出取消，并拒绝后续操作。
func (r *SnapshotRegistry) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	records := make([]*snapshotRecord, 0, len(r.byID))
	for _, record := range r.byID {
		records = append(records, record)
		if record.timer != nil {
			record.timer.Stop()
		}
	}
	r.byID = make(map[string]*snapshotRecord)
	r.byCapability = make(map[string]*snapshotRecord)
	r.mu.Unlock()
	for _, record := range records {
		record.cancel()
	}
}

func (r *SnapshotRegistry) expire(snapshotID string) {
	r.mu.Lock()
	record, ok := r.byID[snapshotID]
	if ok && !r.clock.Now().Before(record.expiresAt) {
		r.removeLocked(record)
	} else {
		ok = false
	}
	r.mu.Unlock()
	if ok {
		record.cancel()
	}
}

func (r *SnapshotRegistry) reapExpired(now time.Time) {
	r.mu.Lock()
	var expired []*snapshotRecord
	for _, record := range r.byID {
		if now.Before(record.expiresAt) {
			continue
		}
		r.removeLocked(record)
		expired = append(expired, record)
	}
	r.mu.Unlock()
	for _, record := range expired {
		record.cancel()
	}
}

func (r *SnapshotRegistry) removeLocked(record *snapshotRecord) {
	delete(r.byID, record.id)
	delete(r.byCapability, record.capability)
	if record.timer != nil {
		record.timer.Stop()
	}
}

func (r *SnapshotRegistry) newIdentity() (string, string, error) {
	var snapshotBytes [16]byte
	if _, err := io.ReadFull(r.entropy, snapshotBytes[:]); err != nil {
		return "", "", err
	}
	snapshotBytes[6] = snapshotBytes[6]&0x0f | 0x40
	snapshotBytes[8] = snapshotBytes[8]&0x3f | 0x80
	hexID := hex.EncodeToString(snapshotBytes[:])
	snapshotID := hexID[:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:]
	var capabilityBytes [32]byte
	if _, err := io.ReadFull(r.entropy, capabilityBytes[:]); err != nil {
		return "", "", err
	}
	return snapshotID, base64.RawURLEncoding.EncodeToString(capabilityBytes[:]), nil
}

func clonePlanSnapshot(snapshot PlanSnapshot) PlanSnapshot {
	cloned := snapshot
	cloned.ExposedBlocks = append([]PlanBlock(nil), snapshot.ExposedBlocks...)
	cloned.Heights = append([]PlanHeight(nil), snapshot.Heights...)
	cloned.ChunkRevisions = append([]pathfind.ChunkRevision(nil), snapshot.ChunkRevisions...)
	cloned.OnlinePlayers = append([]PlanPlayer(nil), snapshot.OnlinePlayers...)
	return cloned
}
