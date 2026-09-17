// Shared low-frequency animation clock, adapted from Waku's GPUI motion helper.
use std::{
    collections::HashMap,
    time::{Duration, Instant},
};

use gpui::{
    Animation, AnimationExt, AnyElement, App, Div, ElementId, EntityId, Global, IntoElement,
    RenderOnce, Styled, Svg, Transformation, Window, ease_in_out, percentage,
    prelude::FluentBuilder, relative,
};

// GPUI currently refreshes the window for each animation frame. A 30 fps loader
// leaves enough frame budget for typing, menus and scrolling while background
// scans are running.
const TICK: Duration = Duration::from_millis(33);
const LEASE: Duration = Duration::from_millis(300);
const SPIN_PERIOD: Duration = Duration::from_millis(900);

struct AnimationLease {
    until: Instant,
}

struct AnimationClock {
    epoch: Instant,
    leases: HashMap<EntityId, AnimationLease>,
    running: bool,
}

impl Global for AnimationClock {}

impl Default for AnimationClock {
    fn default() -> Self {
        Self {
            epoch: Instant::now(),
            leases: HashMap::new(),
            running: false,
        }
    }
}

fn lease(view: EntityId, cx: &mut App) {
    let clock = cx.default_global::<AnimationClock>();
    clock.leases.insert(
        view,
        AnimationLease {
            until: Instant::now() + LEASE,
        },
    );
    if clock.running {
        return;
    }
    clock.running = true;
    cx.spawn(async move |cx| {
        loop {
            cx.background_executor().timer(TICK).await;
            let parked = cx.update(|cx| {
                let clock = cx.default_global::<AnimationClock>();
                let now = Instant::now();
                clock.leases.retain(|_, lease| lease.until > now);
                if clock.leases.is_empty() {
                    clock.running = false;
                    return true;
                }
                for view in clock.leases.keys().copied().collect::<Vec<_>>() {
                    cx.notify(view);
                }
                false
            });
            if parked {
                break;
            }
        }
    })
    .detach();
}

#[derive(IntoElement)]
struct Spinner {
    icon: Svg,
}

impl RenderOnce for Spinner {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let phase = if cx.reduce_motion() {
            0.0
        } else {
            let clock = cx.default_global::<AnimationClock>();
            let phase = (clock.epoch.elapsed().as_secs_f32() / SPIN_PERIOD.as_secs_f32()).fract();
            lease(window.current_view(), cx);
            phase
        };
        self.icon
            .with_transformation(Transformation::rotate(percentage(phase)))
    }
}

pub fn spin(icon: Svg) -> AnyElement {
    Spinner { icon }.into_any_element()
}

/// Vsync-aligned indeterminate fill. Decorative loaders should use this instead
/// of driving `left` from the shared 30 fps clock.
pub fn indeterminate_indicator(id: impl Into<ElementId>, fill: Div, cx: &App) -> AnyElement {
    if cx.reduce_motion() {
        return fill
            .left(relative(0.325))
            .right(relative(0.325))
            .into_any_element();
    }
    fill.with_animation(
        id,
        Animation::new(Duration::from_secs(1)).repeat(),
        |this, delta| {
            let start = relative(ease_in_out(((delta - 0.5) / 0.5).clamp(0., 1.)));
            let end = relative(ease_in_out(1.0 - delta));
            this.when(delta > 0.5, |this| this.left(start)).right(end)
        },
    )
    .into_any_element()
}
