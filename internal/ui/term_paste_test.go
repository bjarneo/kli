package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/vt"
)

func TestTermPasteMessagesQueueText(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{name: "bracketed paste", msg: tea.PasteMsg{Content: "kubectl get pods\n"}, want: "kubectl get pods\n"},
		{name: "clipboard read", msg: tea.ClipboardMsg{Content: "kubectl get ns"}, want: "kubectl get ns"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := make(chan termInput, 1)
			app := App{overlay: overlayTerm, term: termView{input: input}}

			_, cmd := app.Update(tt.msg)
			if cmd != nil {
				t.Fatalf("Update(%T) returned command; want nil", tt.msg)
			}

			select {
			case got := <-input:
				if got.isKey || got.text != tt.want {
					t.Fatalf("queued input = %+v; want text %q", got, tt.want)
				}
			default:
				t.Fatal("paste text was not queued")
			}
		})
	}
}

func TestTermPasteShortcutsRequestClipboard(t *testing.T) {
	input := make(chan termInput, 1)
	app := App{overlay: overlayTerm, term: termView{input: input}}

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl | tea.ModShift})
	if cmd == nil {
		t.Fatal("paste shortcut did not request clipboard")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("clipboard command returned nil message")
	}

	select {
	case got := <-input:
		t.Fatalf("paste shortcut queued input before clipboard response: %+v", got)
	default:
	}
}

func TestTermCtrlVStaysInShell(t *testing.T) {
	input := make(chan termInput, 1)
	app := App{overlay: overlayTerm, term: termView{input: input}}

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("ctrl+v in shell returned command; want terminal input only")
	}

	select {
	case got := <-input:
		if !got.isKey || got.key.Code != 'v' || !got.key.Mod.Contains(vt.ModCtrl) {
			t.Fatalf("queued input = %+v; want ctrl+v key", got)
		}
	default:
		t.Fatal("ctrl+v was not queued for shell")
	}
}

func TestTermCtrlCStaysInShell(t *testing.T) {
	input := make(chan termInput, 1)
	app := App{overlay: overlayTerm, term: termView{input: input}}

	_, cmd := app.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("ctrl+c in shell returned command; want terminal input only")
	}

	select {
	case got := <-input:
		if !got.isKey || got.key.Code != 'c' || !got.key.Mod.Contains(vt.ModCtrl) {
			t.Fatalf("queued input = %+v; want ctrl+c key", got)
		}
	default:
		t.Fatal("ctrl+c was not queued for shell")
	}
}

func TestTermViewReleasesMouseForNativeSelection(t *testing.T) {
	app := App{overlay: overlayTerm}

	if got := app.mouseMode(); got != tea.MouseModeNone {
		t.Fatalf("shell mouse mode = %v; want none", got)
	}
}
