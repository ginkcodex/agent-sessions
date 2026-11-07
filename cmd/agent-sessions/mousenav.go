//go:build desktop

package main

// mouseNavigateEvent carries "back" or "forward" to the frontend when the
// mouse's side buttons are handled natively (see hookMouseNavigation).
const mouseNavigateEvent = "nav:mouse"

// swipeNavigateEvent carries "back" or "forward" for a two-finger touchpad
// swipe on macOS and Linux. Unlike a button, the swipe was also a scroll:
// the frontend drops it when the scroll moved something.
const swipeNavigateEvent = "nav:swipe"
