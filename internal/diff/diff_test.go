package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

func text(p, content string) codepen.File {
	return codepen.File{Path: p, Content: &content}
}

func binary(p string, data []byte) codepen.File {
	sum := sha256.Sum256(data)
	return codepen.File{Path: p, IsBinary: true, ContentHash: hex.EncodeToString(sum[:])}
}

func TestIsSame(t *testing.T) {
	t.Parallel()
	png := []byte{0x89, 'P', 'N', 'G', 0}
	tests := []struct {
		name     string
		local    []byte
		remote   codepen.File
		expected bool
	}{
		{name: "equal text", local: []byte("1"), remote: text("/a.js", "1"), expected: true},
		{name: "changed text", local: []byte("2"), remote: text("/a.js", "1"), expected: false},
		{name: "equal binary by hash", local: png, remote: binary("/a.png", png), expected: true},
		{name: "changed binary", local: []byte{1}, remote: binary("/a.png", png), expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSame(tt.local, tt.remote); got != tt.expected {
				t.Errorf("IsSame() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBetween(t *testing.T) {
	t.Parallel()
	png := []byte{0x89, 'P', 'N', 'G', 0}
	local := tree.Snapshot{
		"/z.js":     []byte("new"),
		"/a.js":     []byte("changed"),
		"/same.js":  []byte("1"),
		"/empty.js": {},
		"/dot.png":  png,
	}
	remote := ByPath([]codepen.File{
		text("/a.js", "old"),
		text("/same.js", "1"),
		text("/gone.js", "x"),
		text("/also-gone.js", "x"),
	})
	expected := codepen.ChangeSet{
		Writes:  []codepen.Write{{Path: "/a.js", Content: "changed"}, {Path: "/z.js", Content: "new"}},
		Deletes: []string{"/also-gone.js", "/gone.js"},
		Uploads: []codepen.Upload{{Path: "/dot.png", Data: png}},
	}
	if got := Between(local, remote); !reflect.DeepEqual(got, expected) {
		t.Errorf("Between() =\n%+v\nwant\n%+v", got, expected)
	}
}

func TestBetween_Nothing(t *testing.T) {
	t.Parallel()
	got := Between(tree.Snapshot{"/a.js": []byte("1")}, ByPath([]codepen.File{text("/a.js", "1")}))
	if !got.IsEmpty() {
		t.Errorf("Between() = %+v, want empty", got)
	}
}
