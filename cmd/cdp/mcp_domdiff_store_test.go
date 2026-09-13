package main

import "testing"

func TestDomSnapshotStoreEvictsOldest(t *testing.T) {
	ds := newDomSnapshotStore()
	for i := 0; i < maxDomSnapshots+5; i++ {
		ds.save(string(rune('a'+i%26))+string(rune('0'+i/26)), &domNode{})
	}
	if got := len(ds.list()); got != maxDomSnapshots {
		t.Errorf("len(list()) = %d, want %d", got, maxDomSnapshots)
	}
	if len(ds.order) != maxDomSnapshots {
		t.Errorf("len(order) = %d, want %d", len(ds.order), maxDomSnapshots)
	}
}

// TestDomSnapshotStoreOverwriteKeepsOneSlot checks that re-saving a name
// refreshes it in place rather than consuming another slot.
func TestDomSnapshotStoreOverwriteKeepsOneSlot(t *testing.T) {
	ds := newDomSnapshotStore()
	want := &domNode{}
	for i := 0; i < maxDomSnapshots*2; i++ {
		ds.save("before", want)
	}
	if got := len(ds.list()); got != 1 {
		t.Errorf("len(list()) = %d, want 1", got)
	}
	if ds.get("before") != want {
		t.Error("get(\"before\") did not return the last saved snapshot")
	}
}
