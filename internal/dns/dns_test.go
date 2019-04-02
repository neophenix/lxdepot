package dns

import "testing"

func TestFindFreeARecord(t *testing.T) {
	var list [256][256][256]int

	// Test 1, make sure we can find a free IP in a simple case where 0 -> 49 are used
	for i := 0; i < 50; i++ {
		list[0][0][i] = 1
	}
	ip, err := findFreeARecord(&list, []string{"10.0.0.2/32,10.0.0.100/32"})
	if err != nil {
		t.FailNow()
	}
	if ip != "10.0.0.50" {
		t.Errorf("T1: Expected 10.0.0.50 got %v", ip)
	}

	// Test 2, could not find a record
	ip, err = findFreeARecord(&list, []string{"10.0.0.2/32, 10.0.0.40/32"})
	if ip != "" {
		t.Errorf("T2: Expected no ip got %v", ip)
	}

	// Test 3, find an IP in a second block passed when the first is used up
	ip, err = findFreeARecord(&list, []string{"10.0.0.2/32,10.0.0.25/32", "10.0.0.40/32, 10.0.0.100/32"})
	if err != nil {
		t.FailNow()
	}
	if ip != "10.0.0.50" {
		t.Errorf("T3: Expected 10.0.0.50 got %v", ip)
	}

	// Test 4, find an IP after exhausting the 3rd octet
	for i := 0; i < 256; i++ {
		list[0][0][i] = 1
	}
	ip, err = findFreeARecord(&list, []string{"10.0.0.2/32,10.0.1.255/32"})
	if err != nil {
		t.FailNow()
	}
	if ip != "10.0.1.1" {
		t.Errorf("T4: Expected 10.0.1.1 got %v", ip)
	}

	// Test 5, find a record where the end block's 4th octet is smaller than the start block's
	// basically the same as above but I figured I might have got this wrong and they can fail in
	// different ways
	ip, err = findFreeARecord(&list, []string{"10.0.0.100/32,10.0.1.40/32"})
	if err != nil {
		t.FailNow()
	}
	if ip != "10.0.1.1" {
		t.Errorf("T4: Expected 10.0.1.1 got %v", ip)
	}
}
