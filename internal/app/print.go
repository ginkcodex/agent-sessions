package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// PrintPageSetup describes the page area shared by print CSS and the native
// dialog. Margins are uniform, in millimetres; scaling is always 100%.
type PrintPageSetup struct {
	Paper       string  `json:"paper"`
	Orientation string  `json:"orientation"`
	MarginMm    float64 `json:"marginMm"`
}

// DefaultPrintPageSetup is the setup used by the argumentless print callback.
func DefaultPrintPageSetup() PrintPageSetup {
	return PrintPageSetup{Paper: "a4", Orientation: "portrait", MarginMm: 14}
}

// UnmarshalJSON requires all fields, including marginMm: an omitted or null
// margin must not silently become a valid zero margin at the binding boundary.
func (s *PrintPageSetup) UnmarshalJSON(data []byte) error {
	var fields struct {
		Paper       *string  `json:"paper"`
		Orientation *string  `json:"orientation"`
		MarginMm    *float64 `json:"marginMm"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields.Paper == nil || fields.Orientation == nil || fields.MarginMm == nil {
		return errors.New("print: paper, orientation and marginMm are required")
	}
	setup := PrintPageSetup{Paper: *fields.Paper, Orientation: *fields.Orientation, MarginMm: *fields.MarginMm}
	if err := setup.validate(); err != nil {
		return err
	}
	*s = setup
	return nil
}

func (s PrintPageSetup) validate() error {
	if s.Paper != "a4" && s.Paper != "letter" {
		return errors.New("print: paper must be a4 or letter")
	}
	if s.Orientation != "portrait" && s.Orientation != "landscape" {
		return errors.New("print: orientation must be portrait or landscape")
	}
	if math.IsNaN(s.MarginMm) || math.IsInf(s.MarginMm, 0) || s.MarginMm < 0 || s.MarginMm > 40 {
		return errors.New("print: marginMm must be a finite number between 0 and 40")
	}
	return nil
}

// PrintWindow opens the system print dialog for the window, where it can
// also be saved as a PDF. The page's print styles decide what is printed.
// macOS needs this: its web view ignores window.print().
func (a *App) PrintWindow() error {
	if a.ctx == nil {
		return errors.New("print: the window is not ready")
	}
	if fn := desktopFrom(a.ctx).Print; fn != nil {
		return fn(a.ctx)
	}
	return fmt.Errorf("print: %w", ErrDesktopUnavailable)
}

// PrintWindowWithSetup opens this window's print dialog with explicit page
// settings. Invalid input never reaches a native callback.
func (a *App) PrintWindowWithSetup(setup PrintPageSetup) error {
	if err := setup.validate(); err != nil {
		return err
	}
	if a.ctx == nil {
		return errors.New("print: the window is not ready")
	}
	if fn := desktopFrom(a.ctx).PrintWithSetup; fn != nil {
		return fn(a.ctx, setup)
	}
	return fmt.Errorf("print: %w", ErrDesktopUnavailable)
}
