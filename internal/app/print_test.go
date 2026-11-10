package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestDefaultPrintPageSetup(t *testing.T) {
	want := PrintPageSetup{Paper: "a4", Orientation: "portrait", MarginMm: 14}
	if got := DefaultPrintPageSetup(); got != want {
		t.Fatalf("default = %+v, want %+v", got, want)
	}
	data, err := json.Marshal(want)
	if err != nil || string(data) != `{"paper":"a4","orientation":"portrait","marginMm":14}` {
		t.Fatalf("setup JSON = %s, %v", data, err)
	}
}

func TestPrintWindowWithSetupValidation(t *testing.T) {
	var got []PrintPageSetup
	ctx := withDesktop(t.Context(), Desktop{PrintWithSetup: func(ctx context.Context, setup PrintPageSetup) error {
		got = append(got, setup)
		return nil
	}})
	a := &App{ctx: ctx}
	for _, paper := range []string{"a4", "letter"} {
		for _, orientation := range []string{"portrait", "landscape"} {
			for _, margin := range []float64{0, 14, 12.5, 40} {
				setup := PrintPageSetup{Paper: paper, Orientation: orientation, MarginMm: margin}
				if err := a.PrintWindowWithSetup(setup); err != nil {
					t.Errorf("valid setup %+v: %v", setup, err)
				} else if got[len(got)-1] != setup {
					t.Errorf("callback got %+v, want %+v", got[len(got)-1], setup)
				}
			}
		}
	}
	before := len(got)
	for _, setup := range []PrintPageSetup{
		{},
		{Paper: "A4", Orientation: "portrait", MarginMm: 14},
		{Paper: "legal", Orientation: "portrait", MarginMm: 14},
		{Paper: "a4", Orientation: "", MarginMm: 14},
		{Paper: "a4", Orientation: "Landscape", MarginMm: 14},
		{Paper: "a4", Orientation: "portrait", MarginMm: -0.1},
		{Paper: "a4", Orientation: "portrait", MarginMm: 40.1},
		{Paper: "a4", Orientation: "portrait", MarginMm: math.NaN()},
		{Paper: "a4", Orientation: "portrait", MarginMm: math.Inf(1)},
		{Paper: "a4", Orientation: "portrait", MarginMm: math.Inf(-1)},
	} {
		if err := a.PrintWindowWithSetup(setup); err == nil {
			t.Errorf("accepted invalid setup %+v", setup)
		}
	}
	if len(got) != before {
		t.Fatal("invalid input reached the desktop callback")
	}
}

func TestPrintPageSetupJSONRequiresExplicitValidFields(t *testing.T) {
	for _, input := range []string{
		`null`, `{}`, `[]`,
		`{"orientation":"portrait","marginMm":14}`,
		`{"paper":"a4","marginMm":14}`,
		`{"paper":"a4","orientation":"portrait"}`,
		`{"paper":null,"orientation":"portrait","marginMm":14}`,
		`{"paper":"a4","orientation":null,"marginMm":14}`,
		`{"paper":"a4","orientation":"portrait","marginMm":null}`,
		`{"paper":"a4","orientation":"portrait","marginMm":"14"}`,
		`{"paper":"legal","orientation":"portrait","marginMm":14}`,
		`{"paper":"a4","orientation":"diagonal","marginMm":14}`,
		`{"paper":"a4","orientation":"portrait","marginMm":-1}`,
		`{"paper":"a4","orientation":"portrait","marginMm":41}`,
		`{"paper":"a4","orientation":"portrait","marginMm":1e999}`,
	} {
		t.Run(input, func(t *testing.T) {
			setup := DefaultPrintPageSetup()
			if err := json.Unmarshal([]byte(input), &setup); err == nil {
				t.Fatalf("accepted %s as %+v", input, setup)
			}
		})
	}
	var setup PrintPageSetup
	if err := json.Unmarshal([]byte(`{"paper":"letter","orientation":"landscape","marginMm":0}`), &setup); err != nil {
		t.Fatal(err)
	}
	if want := (PrintPageSetup{Paper: "letter", Orientation: "landscape", MarginMm: 0}); setup != want {
		t.Fatalf("zero-margin setup = %+v, want %+v", setup, want)
	}
}

func TestPrintCallbacksRemainSeparateAndReturnErrors(t *testing.T) {
	wantErr := errors.New("native print failed")
	var legacy, explicit int
	ctx := withDesktop(t.Context(), Desktop{
		Print: func(context.Context) error {
			legacy++
			return wantErr
		},
		PrintWithSetup: func(context.Context, PrintPageSetup) error {
			explicit++
			return wantErr
		},
	})
	a := &App{ctx: ctx}
	if err := a.PrintWindow(); !errors.Is(err, wantErr) {
		t.Fatalf("PrintWindow = %v", err)
	}
	if err := a.PrintWindowWithSetup(DefaultPrintPageSetup()); !errors.Is(err, wantErr) {
		t.Fatalf("PrintWindowWithSetup = %v", err)
	}
	if legacy != 1 || explicit != 1 {
		t.Fatalf("callback counts = %d, %d", legacy, explicit)
	}
	for _, desktop := range []Desktop{{}, {Print: func(context.Context) error { t.Fatal("must not drop explicit setup"); return nil }}} {
		a := &App{ctx: withDesktop(t.Context(), desktop)}
		if err := a.PrintWindowWithSetup(DefaultPrintPageSetup()); !errors.Is(err, ErrDesktopUnavailable) {
			t.Fatalf("without setup callback = %v", err)
		}
	}
	if err := (&App{}).PrintWindowWithSetup(DefaultPrintPageSetup()); err == nil {
		t.Fatal("unstarted window accepted print")
	}
}

func TestRouterPrintWindowWithSetupRoutesValidatedSetup(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	printed := map[string][]PrintPageSetup{}
	for range 2 {
		name := w.Reserve()
		w.Start(name, t.Context(), Desktop{PrintWithSetup: func(_ context.Context, setup PrintPageSetup) error {
			printed[name] = append(printed[name], setup)
			return nil
		}})
	}
	first := DefaultPrintPageSetup()
	second := PrintPageSetup{Paper: "letter", Orientation: "landscape", MarginMm: 0}
	for _, call := range []struct {
		window string
		setup  PrintPageSetup
	}{{"window-1", first}, {"window-2", second}, {"window-2", first}} {
		if _, err := r.Call(fromWindow(call.window), "PrintWindowWithSetup", rawArgs(t, call.setup)); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string][]PrintPageSetup{"window-1": {first}, "window-2": {second, first}}
	if !reflect.DeepEqual(printed, want) {
		t.Fatalf("prints = %+v, want %+v", printed, want)
	}
	for _, raw := range []string{
		`{"paper":"a4","orientation":"portrait"}`,
		`{"paper":"a4","orientation":"portrait","marginMm":41}`,
		`null`,
	} {
		if _, err := r.Call(fromWindow("window-2"), "PrintWindowWithSetup", []json.RawMessage{json.RawMessage(raw)}); err == nil {
			t.Fatalf("router accepted %s", raw)
		}
	}
	if !reflect.DeepEqual(printed, want) {
		t.Fatal("invalid routed setup reached a window")
	}
	if _, err := r.Call(fromWindow("window-1"), "PrintWindowWithSetup", nil); err == nil {
		t.Fatal("router accepted missing setup argument")
	}
}
