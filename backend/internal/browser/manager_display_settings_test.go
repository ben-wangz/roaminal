package browser

import "testing"

func TestInitialViewportResizeSynchronizesNewPrimaryAtDefaultSize(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-1", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready"}
	m := newTestManager(runtime)
	current := newTestViewer(runtime, "client-one")
	m.addViewer(current)
	readEvent(t, current)

	if err := m.command(current, []byte(`{"type":"resize","clientId":"client-one","generation":"generation-1","width":1280,"height":720,"primaryIntent":true}`)); err != nil {
		t.Fatalf("initial resize: %v", err)
	}
	if writer.count() != 1 {
		t.Fatalf("initial viewport sync writes = %d, want 1", writer.count())
	}
	if m.primary != current.clientID || !m.primaryKnown {
		t.Fatalf("initial primary = %q, known=%v", m.primary, m.primaryKnown)
	}
	for {
		event := readEvent(t, current)
		if event["type"] == "resize_accepted" {
			if event["width"] != float64(1280) || event["height"] != float64(720) {
				t.Fatalf("initial resize response = %#v", event)
			}
			break
		}
	}
}

func TestDisplaySettingsArePrimaryControlledAndFixViewport(t *testing.T) {
	writer := &recordingWriter{}
	runtime := &process{stdin: writer, generation: "generation-current", viewport: viewportSize{Width: 1280, Height: 720}, pageStatus: "ready"}
	m := newTestManager(runtime)
	primary := newTestViewer(runtime, "client-one")
	secondary := newTestViewer(runtime, "client-two")
	m.addViewer(primary)
	m.addViewer(secondary)
	readEvent(t, primary)
	readEvent(t, secondary)

	fixedSettings := `{"type":"settings","clientId":"client-one","generation":"generation-current","primaryIntent":true,"viewportMode":"fixed","width":1440,"height":900,"frameRate":5,"quality":40}`
	if err := m.command(primary, []byte(fixedSettings)); err != nil {
		t.Fatalf("primary display settings: %v", err)
	}
	if runtime.viewport != (viewportSize{Width: 1440, Height: 900}) || runtime.displaySettings() != (browserDisplaySettings{viewportMode: "fixed", frameRate: 5, quality: 40}) {
		t.Fatalf("applied display settings = viewport %#v, settings %#v", runtime.viewport, runtime.displaySettings())
	}
	if writer.count() != 1 {
		t.Fatalf("settings worker writes = %d, want 1", writer.count())
	}

	secondarySettings := `{"type":"settings","clientId":"client-two","generation":"generation-current","primaryIntent":true,"viewportMode":"fixed","width":1920,"height":1080,"frameRate":15,"quality":70}`
	if err := m.command(secondary, []byte(secondarySettings)); err != nil {
		t.Fatalf("secondary display settings should be rejected on the stream: %v", err)
	}
	for {
		event := readEvent(t, secondary)
		if event["type"] == "settings_rejected" {
			if event["code"] != "not_primary_client" || event["width"] != float64(1440) || event["height"] != float64(900) {
				t.Fatalf("secondary rejection = %#v", event)
			}
			break
		}
	}
	if writer.count() != 1 || runtime.viewport != (viewportSize{Width: 1440, Height: 900}) {
		t.Fatalf("secondary settings changed browser state: writes=%d viewport=%#v", writer.count(), runtime.viewport)
	}

	if err := m.command(primary, []byte(`{"type":"resize","clientId":"client-one","generation":"generation-current","width":1920,"height":1080,"primaryIntent":true}`)); err != nil {
		t.Fatalf("stale automatic resize should be accepted without changing fixed size: %v", err)
	}
	for {
		event := readEvent(t, primary)
		if event["type"] == "resize_accepted" {
			if event["width"] != float64(1440) || event["height"] != float64(900) || event["viewportMode"] != "fixed" {
				t.Fatalf("fixed viewport resize response = %#v", event)
			}
			break
		}
	}
	if writer.count() != 1 || runtime.viewport != (viewportSize{Width: 1440, Height: 900}) {
		t.Fatalf("automatic resize changed fixed viewport: writes=%d viewport=%#v", writer.count(), runtime.viewport)
	}
}
