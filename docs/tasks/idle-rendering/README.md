# Idle rendering — tasks

- **Plan:** [idle-rendering.md](../../plan/idle-rendering.md)
- **Progress:** 3/7 done · 4 🟡 (attribution, controlled scrolling/opacity comparison and native QA outstanding)

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [IR-01](IR-01.md) | ✅ Done | Static liveness and platform-scoped list compositing | 1 h | — |
| [IR-02](IR-02.md) | ✅ Done | Visibility-aware spinners and animation lifecycle policies | 3 h | IR-01 |
| [IR-03](IR-03.md) | 🟡 Partial — native QA outstanding | Automated validation and native cross-platform measurement record | 2 h | IR-01, IR-02 |
| [IR-04](IR-04.md) | ✅ Done | Side-effect review: drop global pauses, restore reduced-motion spinner | 2 h | IR-01, IR-02 |
| [IR-05](IR-05.md) | 🟡 Partial — startup attribution and controlled comparison outstanding | Attribute the remaining macOS startup and steady GPU load | 3 h | IR-04 |
| [IR-06](IR-06.md) | 🟡 Partial — implementation/tests complete; native comparison outstanding | Low-rate visible work indicators | 3 h | IR-05 |
| [IR-07](IR-07.md) | 🟡 Partial — user reports 0% idle GPU; controlled GPU A/B and visual QA outstanding | Scrolling and native-window compositing cost | 3 h | IR-06 |
