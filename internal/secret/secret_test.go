package secret

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()

	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.Entries == nil {
		t.Error("Entries map is nil")
	}

	if s.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
}

func TestSerializeDeserialize(t *testing.T) {
	s := New()
	s.Entries["KEY1"] = "value1"
	s.Entries["KEY2"] = "value2"

	// Serialize
	data, err := s.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Deserialize
	s2, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if len(s2.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(s2.Entries))
	}

	if s2.Entries["KEY1"] != "value1" {
		t.Errorf("Expected value1, got %s", s2.Entries["KEY1"])
	}

	if s2.Entries["KEY2"] != "value2" {
		t.Errorf("Expected value2, got %s", s2.Entries["KEY2"])
	}
}

func TestAppend(t *testing.T) {
	s1 := New()
	s1.Entries["KEY1"] = "value1"

	s2 := New()
	s2.Entries["KEY2"] = "value2"

	s3 := s1.Append(s2)

	if len(s3.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(s3.Entries))
	}

	if s3.Entries["KEY1"] != "value1" {
		t.Errorf("Expected value1, got %s", s3.Entries["KEY1"])
	}

	if s3.Entries["KEY2"] != "value2" {
		t.Errorf("Expected value2, got %s", s3.Entries["KEY2"])
	}
}

func TestAppendOverwrite(t *testing.T) {
	s1 := New()
	s1.Entries["KEY1"] = "value1"
	s1.Entries["KEY2"] = "oldvalue"

	s2 := New()
	s2.Entries["KEY2"] = "newvalue"

	s3 := s1.Append(s2)

	if s3.Entries["KEY2"] != "newvalue" {
		t.Errorf("Expected newvalue, got %s", s3.Entries["KEY2"])
	}
}

func TestIsExpired(t *testing.T) {
	s := New()
	s.Timestamp = time.Now().Add(-2 * time.Hour)

	// Should be expired with 1 hour TTL
	if !s.IsExpired(1 * time.Hour) {
		t.Error("Expected secrets to be expired")
	}

	// Should not be expired with 3 hour TTL
	if s.IsExpired(3 * time.Hour) {
		t.Error("Expected secrets to not be expired")
	}
}

func TestIsNotExpired(t *testing.T) {
	s := New()

	// Should not be expired (just created)
	if s.IsExpired(1 * time.Hour) {
		t.Error("Expected secrets to not be expired")
	}
}
