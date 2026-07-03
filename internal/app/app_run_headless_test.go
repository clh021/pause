package app

import "testing"

func TestNewHeadlessAppUsesNoopDesktopController(t *testing.T) {
	app, err := NewHeadlessApp(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatalf("NewHeadlessApp() err=%v", err)
	}
	t.Cleanup(func() {
		app.Shutdown(nil)
	})

	if _, ok := app.desktop.(noopDesktopController); !ok {
		t.Fatalf("expected noopDesktopController, got %T", app.desktop)
	}
}

func TestRunQuitFunc(t *testing.T) {
	app := &App{}
	called := false
	app.setQuitFunc(func() {
		called = true
	})

	if !app.runQuitFunc() {
		t.Fatalf("expected runQuitFunc()=true")
	}
	if !called {
		t.Fatalf("expected quit func to be called")
	}

	app.setQuitFunc(nil)
	if app.runQuitFunc() {
		t.Fatalf("expected runQuitFunc()=false when quit func is nil")
	}
}
