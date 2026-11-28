# Native-tone dark surfaces

Planned 2026-10-24. Tasks: [dark-tone](../tasks/dark-tone/README.md).

Target: #2C2C2B, matching raw screenshot title-bar pixels RGB (44,44,43).
The initial choice matched all large surfaces. The user clarified that dark mode
should retain lighter/darker surface layers, equivalent to the light-mode hierarchy.
The native title bar remains OS-managed; its appearance can vary with focus and
system settings.

2026-11-28 ([DT-06](../tasks/dark-tone/DT-06.md)): the user found dark mode too
dark once the pane gutter arrived. Every dark surface moved 6 steps lighter per
channel, keeping the layers: gutter #20201F, primary #2A2A29, secondary
#323231, tertiary #3E3E3D. The secondary surface is now slightly lighter than
the title-bar target above.
