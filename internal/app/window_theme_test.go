package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSetWindowTheme(t *testing.T) {
	wantErr := errors.New("native failure")
	var calls []string
	ctx := withDesktop(context.Background(), Desktop{
		SetWindowTheme: func(_ context.Context, mode string) error {
			calls = append(calls, mode)
			if mode == "dark" {
				return wantErr
			}
			return nil
		},
	})
	a := &App{ctx: ctx}
	for _, mode := range []string{"system", "light", "dark"} {
		err := a.SetWindowTheme(mode)
		if mode == "dark" {
			if !errors.Is(err, wantErr) {
				t.Fatalf("dark error = %v, want %v", err, wantErr)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"", "Dark", "auto", " light", "dark\x00"} {
		if err := a.SetWindowTheme(mode); err == nil {
			t.Fatalf("accepted invalid mode %q", mode)
		}
	}
	if !reflect.DeepEqual(calls, []string{"system", "light", "dark"}) {
		t.Fatalf("callback modes = %v", calls)
	}
}

func TestSetWindowThemeWithoutDesktop(t *testing.T) {
	for _, a := range []*App{{}, {ctx: context.Background()}} {
		if err := a.SetWindowTheme("light"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetWindowThemeIsWindowLocal(t *testing.T) {
	var first, second string
	a := &App{ctx: withDesktop(context.Background(), Desktop{
		SetWindowTheme: func(_ context.Context, mode string) error { first = mode; return nil },
	})}
	b := &App{ctx: withDesktop(context.Background(), Desktop{
		SetWindowTheme: func(_ context.Context, mode string) error { second = mode; return nil },
	})}
	if err := a.SetWindowTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if first != "dark" || second != "" {
		t.Fatalf("first=%q second=%q", first, second)
	}
	if err := b.SetWindowTheme("light"); err != nil {
		t.Fatal(err)
	}
	if first != "dark" || second != "light" {
		t.Fatalf("first=%q second=%q", first, second)
	}
}
