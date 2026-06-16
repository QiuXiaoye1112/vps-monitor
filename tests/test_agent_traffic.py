from datetime import datetime, timezone

from agent import (
    TRAFFIC_STATE_VERSION,
    load_traffic_state,
    save_traffic_state,
    traffic_cycle_key,
    update_monthly_traffic,
)


def config(reset_day: int = 1, reset_hour: int = 0, reset_minute: int = 0) -> dict[str, object]:
    return {
        "traffic_reset_day": reset_day,
        "traffic_reset_hour": reset_hour,
        "traffic_reset_minute": reset_minute,
    }


def test_cycle_uses_previous_month_before_beijing_reset_time() -> None:
    now = datetime(2026, 6, 14, 19, 0, tzinfo=timezone.utc)
    assert traffic_cycle_key(now, 15, 4).startswith("2026-05-15T04:00:00+08:00")


def test_reset_day_is_clamped_to_last_day_of_short_month() -> None:
    now = datetime(2026, 2, 28, 23, 0, tzinfo=timezone.utc)
    assert traffic_cycle_key(now, 31, 4).startswith("2026-02-28T04:00:00")


def test_monthly_usage_survives_counter_reset_after_reboot() -> None:
    state: dict[str, object] = {}
    now = datetime(2026, 6, 15, 5, 0, tzinfo=timezone.utc)
    update_monthly_traffic({"bytes_sent": 1000, "bytes_recv": 2000}, config(15, 4), state, now=now)
    update_monthly_traffic({"bytes_sent": 1600, "bytes_recv": 2600}, config(15, 4), state, now=now)
    update_monthly_traffic({"bytes_sent": 100, "bytes_recv": 200}, config(15, 4), state, now=now)
    assert state["tx"] == 700
    assert state["rx"] == 800


def test_missing_reset_time_accumulates_without_changing_cycle() -> None:
    state: dict[str, object] = {}
    no_reset = config(0, 0)
    update_monthly_traffic({"bytes_sent": 1000, "bytes_recv": 2000}, no_reset, state)
    update_monthly_traffic({"bytes_sent": 1500, "bytes_recv": 2600}, no_reset, state)
    assert state["cycle"] == "never"
    assert state["tx"] == 500
    assert state["rx"] == 600


def test_missing_reset_time_can_still_have_a_traffic_limit() -> None:
    state: dict[str, object] = {}
    no_reset_with_limit = {**config(0, 0), "traffic_limit_gb": 10.0}
    update_monthly_traffic(
        {"bytes_sent": 1000, "bytes_recv": 2000},
        no_reset_with_limit,
        state,
    )
    update_monthly_traffic(
        {"bytes_sent": 1500, "bytes_recv": 2600},
        no_reset_with_limit,
        state,
    )
    assert state["cycle"] == "never"
    assert no_reset_with_limit["traffic_limit_gb"] == 10.0
    assert state["tx"] + state["rx"] == 1100


def test_new_cycle_starts_from_zero() -> None:
    state: dict[str, object] = {}
    first_cycle = datetime(2026, 6, 15, 5, 0, tzinfo=timezone.utc)
    next_cycle = datetime(2026, 7, 15, 5, 0, tzinfo=timezone.utc)

    update_monthly_traffic(
        {"bytes_sent": 1000, "bytes_recv": 2000},
        config(15, 4),
        state,
        now=first_cycle,
    )
    assert state["tx"] + state["rx"] == 0

    update_monthly_traffic(
        {"bytes_sent": 3000, "bytes_recv": 4000},
        config(15, 4),
        state,
        now=next_cycle,
    )
    assert state["tx"] + state["rx"] == 0


def test_old_agent_traffic_state_is_discarded() -> None:
    now = datetime(2026, 6, 15, 5, 0, tzinfo=timezone.utc)
    state: dict[str, object] = {
        "cycle": traffic_cycle_key(now, 15, 4),
        "last_sent": 1000,
        "last_recv": 2000,
        "tx": 50 * 1073741824,
        "rx": 50 * 1073741824,
    }

    update_monthly_traffic(
        {"bytes_sent": 3000, "bytes_recv": 4000},
        config(15, 4),
        state,
        now=now,
    )

    assert state["version"] == TRAFFIC_STATE_VERSION
    assert state["tx"] + state["rx"] == 0


def test_traffic_state_round_trip(tmp_path) -> None:
    path = tmp_path / "traffic-state.json"
    save_traffic_state(path, {"cycle": "cycle-1", "tx": 12, "rx": 34, "_save_after": 99})
    assert load_traffic_state(path) == {"cycle": "cycle-1", "tx": 12, "rx": 34}
