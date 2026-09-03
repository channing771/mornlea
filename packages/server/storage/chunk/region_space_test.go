package chunk

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage/region"
)

// TestRegionSaveReusesInactiveOnlyExtentWithoutGrowing 随记录层容器落 chunk 包：
// 经 openRegion 装配容器，断言 revision 推进优先复用非生效 bank 的独占 extent
// 且不增长文件，与基线断言逐字一致。
func TestRegionSaveReusesInactiveOnlyExtentWithoutGrowing(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	r, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	_, slot := region.RegionFor(chunkKey)
	inactiveOnly := r.bank.Entries[slot]
	if _, err := r.Save(context.Background(), []ChunkSave{changedSave(chunkKey, 2)}); err != nil {
		t.Fatal(err)
	}
	activeBefore := r.bank.Entries[slot]
	if regionExtentsOverlap(activeBefore, inactiveOnly) {
		t.Fatalf("fixture extents overlap: active=%+v inactive-only=%+v", activeBefore, inactiveOnly)
	}
	infoBefore, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Save(context.Background(), []ChunkSave{changedSave(chunkKey, 3)}); err != nil {
		t.Fatal(err)
	}
	got := r.bank.Entries[slot]
	if got.OffsetSector != inactiveOnly.OffsetSector || got.SectorCount != inactiveOnly.SectorCount {
		t.Fatalf("revision 3 extent = %+v, want inactive-only extent %+v", got, inactiveOnly)
	}
	if regionExtentsOverlap(got, activeBefore) {
		t.Fatalf("revision 3 overwrote active extent: new=%+v active=%+v", got, activeBefore)
	}
	infoAfter, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if infoAfter.Size() != infoBefore.Size() {
		t.Fatalf("region grew from %d to %d while reusable extent existed", infoBefore.Size(), infoAfter.Size())
	}
}
