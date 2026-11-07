package core

import (
	"strings"
	"testing"
)

func TestLogsBoundedAndCursors(t *testing.T) {
	var ring Ring
	for i := 0; i < 5000; i++ {
		ring.Add("info", "test", strings.Repeat("x", 2048))
	}
	items := ring.Since(0)
	if len(items) > 4000 || ring.bytes > 4<<20 {
		t.Fatal("log buffer exceeded its budget")
	}
	cursor := items[len(items)-1].Sequence
	ring.Add("error", "test", "last")
	next := ring.Since(cursor)
	if len(next) != 1 || next[0].Message != "last" {
		t.Fatal("cursor did not return new log")
	}
}

func TestProcessOutputKeepsFragmentsAndBoundsLongLines(t *testing.T) {
	var ring Ring
	writer := processOutput{logs: &ring}
	after := ring.Cursor()
	_, _ = writer.Write([]byte(`level=error msg="Start TUN `))
	_, _ = writer.Write([]byte("listening error\"\nlevel=info msg=ready\nlast line without newline"))
	writer.flush()
	items := ring.Since(after)
	if len(items) != 3 || items[0].Level != "error" || !strings.Contains(items[0].Message, "Start TUN listening error") || items[2].Message != "last line without newline" {
		t.Fatalf("fragmented output lost: %#v", items)
	}
	_, _ = writer.Write([]byte(strings.Repeat("x", 1<<20)))
	writer.flush()
	for _, entry := range ring.Since(after) {
		if len(entry.Message) > 32<<10 {
			t.Fatal("unbounded output line")
		}
	}
}
