package blobstore

import "testing"

func TestSafeName(t *testing.T) {
	tests := map[string]string{
		"../report.pdf": "report.pdf",
		`..\report.pdf`: "report.pdf",
		"a:b*c?.txt":    "a_b_c_.txt",
		"   .   ":       "file",
	}
	for in, want := range tests {
		if got := SafeName(in); got != want {
			t.Fatalf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}
