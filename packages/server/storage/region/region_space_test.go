package region

import "testing"

func TestAllocatorNeverUsesActiveExtentsAndUsesFirstFit(t *testing.T) {
	bank := Bank{Generation: 4}
	bank.Entries[0] = Entry{
		OffsetSector:  15,
		SectorCount:   2,
		PayloadLength: 5000,
		Revision:      1,
	}
	bank.Entries[1] = Entry{
		OffsetSector:  20,
		SectorCount:   1,
		PayloadLength: 100,
		Revision:      1,
	}

	free, err := FreeSectorExtents(bank, 24*SectorSize)
	if err != nil {
		t.Fatal(err)
	}
	wantFree := []Extent{{First: 17, Count: 3}, {First: 21, Count: 3}}
	if len(free) != len(wantFree) {
		t.Fatalf("free extents = %+v, want %+v", free, wantFree)
	}
	for index := range wantFree {
		if free[index] != wantFree[index] {
			t.Fatalf("free extents = %+v, want %+v", free, wantFree)
		}
	}

	extent, remaining := AllocateExtent(free, 3, 24)
	if extent != (Extent{First: 17, Count: 3}) {
		t.Fatalf("extent = %+v, want first free fit", extent)
	}
	if len(remaining) != 1 || remaining[0] != (Extent{First: 21, Count: 3}) {
		t.Fatalf("remaining extents = %+v, want second free run", remaining)
	}
}

func TestAllocatorAppendsOnlyWhenNoFreeExtentFits(t *testing.T) {
	free := []Extent{{First: 16, Count: 1}, {First: 20, Count: 2}}

	extent, remaining := AllocateExtent(free, 3, 24)
	if extent != (Extent{First: 24, Count: 3}) {
		t.Fatalf("extent = %+v, want append extent", extent)
	}
	if len(remaining) != len(free) || remaining[0] != free[0] || remaining[1] != free[1] {
		t.Fatalf("remaining extents = %+v, want unchanged %+v", remaining, free)
	}
}
