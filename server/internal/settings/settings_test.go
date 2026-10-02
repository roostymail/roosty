package settings

import (
	"testing"

	"github.com/roostymail/roosty/server/internal/store"
)

func TestAllowed(t *testing.T) {
	s := Settings{Access: Access{Mode: "domains", Domains: []string{"roosty.dev"}}}
	if !s.Allowed("Ana@Roosty.dev") || s.Allowed("ana@roosty.dev.evil.com") || s.Allowed("x@gmail.com") {
		t.Error("domain mode")
	}
	s.Access = Access{Mode: "list", Accounts: []string{"ana@roosty.dev"}}
	if !s.Allowed("ana@roosty.dev") || s.Allowed("bob@roosty.dev") {
		t.Error("list mode")
	}
}

func TestManagerGet_ReturnsDeepCopy(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m, _ := NewManager(st)
	s := Defaults()
	s.Access.Domains = []string{"roosty.dev"}
	if err := m.Save(s); err != nil {
		t.Fatal(err)
	}
	got := m.Get()
	got.Access.Domains[0] = "evil.com"
	got.Branding.Themes[0] = "x"
	if m.Get().Access.Domains[0] != "roosty.dev" || m.Get().Branding.Themes[0] != "claro" {
		t.Fatal("Get must not expose live slices")
	}
}
