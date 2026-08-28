package region

import (
	"fmt"
	"math"

	"github.com/channing771/mornlea/internal/storage/storagedef"
)

// Extent 是一段连续扇区区间：freeSectorExtents 的空闲区间与 allocateExtent
// 的分配结果共用这一值类型。
type Extent struct {
	First uint32
	Count uint32
}

// SpacePolicy 是 region 压缩判定策略：浪费字节数须同时达到绝对下限
// （MinWaste）与相对比例（WasteRatio）才值得整文件重写。
type SpacePolicy struct {
	WasteRatio float64
	MinWaste   int64
}

// ProductionSpacePolicy 是权威保存路径使用的生产压缩策略。暴露为可变量以
// 保持拆分前语义：根包编排测试在同进程内替换它来触达压缩路径。
var ProductionSpacePolicy = SpacePolicy{
	WasteRatio: 0.25,
	MinWaste:   8 << 20,
}

// CompactionHooks 是 region 压缩替换路径的故障注入点：三个钩子分别对应
// 临时文件 sync 前、临时文件改名与目录 sync。生产路径全部为 nil，
// 由容器侧取默认实现。
type CompactionHooks struct {
	BeforeTempSync func() error
	Rename         func(string, string) error
	SyncDirectory  func(string) error
}

// FreeSectorExtents 按 bank 索引与文件大小枚举数据区内的空闲扇区区间，
// 供 chunk 记录层在写入前做首选适配分配。
func FreeSectorExtents(bank Bank, fileSize int64) ([]Extent, error) {
	if fileSize < int64(DataStartSector*SectorSize) {
		return nil, fmt.Errorf("%w: region file is shorter than fixed headers", storagedef.ErrCorrupt)
	}
	if err := validateRegionBank(bank, fileSize, true); err != nil {
		return nil, err
	}

	totalSectors := uint64(fileSize / SectorSize)
	if fileSize%SectorSize != 0 {
		totalSectors++
	}
	if totalSectors > math.MaxUint32 {
		return nil, fmt.Errorf("%w: region file exceeds uint32 sectors", storagedef.ErrCorrupt)
	}
	occupied := make([]bool, int(totalSectors))
	for sector := 0; sector < DataStartSector; sector++ {
		occupied[sector] = true
	}
	for _, entry := range bank.Entries {
		if entry.OffsetSector == 0 {
			continue
		}
		end := uint64(entry.OffsetSector) + uint64(entry.SectorCount)
		for sector := uint64(entry.OffsetSector); sector < end; sector++ {
			occupied[int(sector)] = true
		}
	}

	free := make([]Extent, 0)
	for sector := uint32(DataStartSector); uint64(sector) < totalSectors; {
		if occupied[int(sector)] {
			sector++
			continue
		}
		first := sector
		for uint64(sector) < totalSectors && !occupied[int(sector)] {
			sector++
		}
		free = append(free, Extent{First: first, Count: sector - first})
	}
	return free, nil
}

// AllocateExtent 在空闲区间上做首选适配：命中即原地切分；无可用区间时
// 返回文件末尾的追加区间。返回值 remaining 是扣除本次分配后的空闲表，
// 调用方在同批次内继续复用。
func AllocateExtent(
	free []Extent,
	sectorCount uint32,
	appendSector uint32,
) (Extent, []Extent) {
	remaining := append([]Extent(nil), free...)
	if sectorCount == 0 {
		return Extent{}, remaining
	}
	for index, candidate := range remaining {
		if candidate.Count < sectorCount {
			continue
		}
		allocated := Extent{First: candidate.First, Count: sectorCount}
		if candidate.Count == sectorCount {
			remaining = append(remaining[:index], remaining[index+1:]...)
		} else {
			remaining[index].First += sectorCount
			remaining[index].Count -= sectorCount
		}
		return allocated, remaining
	}
	if uint64(appendSector)+uint64(sectorCount) > math.MaxUint32 {
		return Extent{}, remaining
	}
	return Extent{First: appendSector, Count: sectorCount}, remaining
}
