package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCandidateCursorDefaultAndExplicitChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s, err := Load(path)
	if err != nil || s.CandidateCursor != "after" {
		t.Fatalf("%+v %v", s, err)
	}
	// Pre-existing settings without the field also use the default.
	if err := json.Unmarshal([]byte(`{"schema":1,"selection":"statistical","future":7}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.CandidateCursor != Default().CandidateCursor || s.Selection != Statistical {
		t.Fatalf("%+v", s)
	}
	for _, value := range []string{"after", "before", "after"} {
		if err := s.Set("candidate-cursor", value); err != nil {
			t.Fatal(err)
		}
		if err := Save(path, s); err != nil {
			t.Fatal(err)
		}
		s, err = Load(path)
		if err != nil || s.CandidateCursor != value {
			t.Fatalf("%+v %v", s, err)
		}
		b, _ := json.Marshal(s)
		var raw map[string]any
		json.Unmarshal(b, &raw)
		if raw["future"] != float64(7) {
			t.Fatal("unknown setting lost")
		}
	}
	if err := s.Set("candidate-cursor", "left"); err == nil {
		t.Fatal("invalid value accepted")
	}
}

func TestCandidateCursorUnknownValueAndNewerSchema(t *testing.T) {
	for _, value := range []string{`"future-direction"`, `false`, `null`} {
		var s Settings
		if err := json.Unmarshal([]byte(`{"candidate_cursor":`+value+`}`), &s); err != nil {
			t.Fatal(err)
		}
		if s.CandidateCursor != Default().CandidateCursor || len(s.notes()) == 0 {
			t.Fatalf("%+v", s)
		}
		b, _ := json.Marshal(s)
		var raw map[string]json.RawMessage
		json.Unmarshal(b, &raw)
		if string(raw["candidate_cursor"]) != value {
			t.Fatalf("unknown value lost: %s", b)
		}
		if err := s.Set("candidate-cursor", "before"); err != nil {
			t.Fatal(err)
		}
		b, _ = json.Marshal(s)
		json.Unmarshal(b, &raw)
		if string(raw["candidate_cursor"]) != `"before"` {
			t.Fatalf("explicit choice not saved: %s", b)
		}
	}
	var s Settings
	json.Unmarshal([]byte(`{"schema":9,"candidate_cursor":"before"}`), &s)
	if !s.ReadOnly() || Save(filepath.Join(t.TempDir(), FileName), s) != ErrReadOnly {
		t.Fatal("newer schema must remain read-only")
	}
}
