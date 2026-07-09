#!/usr/bin/env bash
# fipindicateur-watchdog: kills fipindicateur if the GNOME session looks wedged.
#
# Context: on 2026-07-09 a tray icon/metadata flood (SNI churn) drove the
# appindicator extension into a loop and froze gnome-shell. The app is fixed
# (single flock instance, no icon churn), but belt and braces: while
# fipindicateur runs, this watchdog probes the session and shoots the radio,
# never the shell.
#
# Tripwires (only ever kills fipindicateur, and only while it is running):
#   1. gnome-shell stops answering a D-Bus property read (dispatched on its
#      main loop, so a wedged shell times out even at low CPU).
#   2. gnome-shell CPU pegged for STRIKES consecutive samples.
#   3. fipindicateur itself spinning CPU for STRIKES consecutive samples.
#
# All thresholds overridable via FIPWD_* env vars (used by tests).
set -u

INTERVAL=${FIPWD_INTERVAL:-2}          # seconds between samples
SHELL_TRIP=${FIPWD_SHELL_TRIP:-85}     # gnome-shell CPU% considered pegged
FIP_TRIP=${FIPWD_FIP_TRIP:-60}         # fipindicateur CPU% considered spinning
STRIKES=${FIPWD_STRIKES:-6}            # consecutive bad samples before killing
PING_TIMEOUT=${FIPWD_PING_TIMEOUT:-3}  # seconds before a shell D-Bus read counts as wedged
PING_STRIKES=${FIPWD_PING_STRIKES:-2}  # consecutive timeouts before killing

LOGDIR=${XDG_DATA_HOME:-$HOME/.local/share}/fipindicateur
LOGFILE=$LOGDIR/watchdog.log
CLK=$(getconf CLK_TCK)

log() {
	mkdir -p "$LOGDIR"
	printf '%s %s\n' "$(date -Is)" "$*" >>"$LOGFILE"
}

# jiffies PID: total utime+stime of a process, or fail if it is gone.
# /proc/PID/stat embeds the comm in parens; strip through the closing paren so
# field numbering is stable even if the name held spaces.
jiffies() {
	local s
	s=$(cat "/proc/$1/stat" 2>/dev/null) || return 1
	s=${s##*) }
	# shellcheck disable=SC2086
	set -- $s
	echo $((${12} + ${13}))
}

# A property read on org.gnome.Shell is dispatched on the shell main loop:
# it hangs iff the session is wedged, which is exactly what we want to detect.
shell_ping_ok() {
	timeout "$PING_TIMEOUT" gdbus call --session \
		--dest org.gnome.Shell --object-path /org/gnome/Shell \
		--method org.freedesktop.DBus.Properties.Get org.gnome.Shell ShellVersion \
		>/dev/null 2>&1
}

kill_fip() {
	local reason=$1
	log "TRIP: $reason · killing fipindicateur"
	notify-send -a fipindicateur -u critical "fipindicateur arrete par le watchdog" "$reason" 2>/dev/null || true
	pkill -TERM -x fipindicateur 2>/dev/null || true
	for _ in 1 2 3 4 5 6; do
		if ! pgrep -x fipindicateur >/dev/null; then
			log "fipindicateur terminated cleanly"
			return
		fi
		sleep 0.5
	done
	pkill -KILL -x fipindicateur 2>/dev/null || true
	log "fipindicateur SIGKILLed"
}

shell_strikes=0 fip_strikes=0 ping_strikes=0
prev_shell_pid="" prev_fip_pid="" prev_shell_j=0 prev_fip_j=0 prev_t=0

log "watchdog started (interval=${INTERVAL}s shell_trip=${SHELL_TRIP}% fip_trip=${FIP_TRIP}% strikes=${STRIKES} ping=${PING_STRIKES}x${PING_TIMEOUT}s)"

while :; do
	fip_pid=$(pgrep -o -x fipindicateur || true)
	if [ -z "$fip_pid" ]; then
		# Nothing to guard: idle cheaply and forget all history.
		shell_strikes=0 fip_strikes=0 ping_strikes=0
		prev_shell_pid="" prev_fip_pid=""
		sleep 5
		continue
	fi

	now_t=$(date +%s)
	shell_pid=$(pgrep -o -x gnome-shell || true)

	if [ -n "$shell_pid" ]; then
		if shell_ping_ok; then
			ping_strikes=0
		else
			ping_strikes=$((ping_strikes + 1))
			log "gnome-shell D-Bus read timed out ($ping_strikes/$PING_STRIKES)"
			if [ "$ping_strikes" -ge "$PING_STRIKES" ]; then
				kill_fip "gnome-shell ne repond plus sur D-Bus"
				ping_strikes=0 shell_strikes=0 fip_strikes=0
				sleep "$INTERVAL"
				continue
			fi
		fi
	fi

	# CPU% per process over the real elapsed window (the ping probe can add
	# up to PING_TIMEOUT to a loop turn, so never assume dt == INTERVAL).
	dt=$((now_t - prev_t))
	[ "$dt" -lt 1 ] && dt=1

	if [ -n "$shell_pid" ] && [ "$shell_pid" = "$prev_shell_pid" ]; then
		if j=$(jiffies "$shell_pid"); then
			pct=$(((j - prev_shell_j) * 100 / (CLK * dt)))
			prev_shell_j=$j
			if [ "$pct" -ge "$SHELL_TRIP" ]; then
				shell_strikes=$((shell_strikes + 1))
				log "gnome-shell at ${pct}% CPU ($shell_strikes/$STRIKES)"
			else
				shell_strikes=0
			fi
		fi
	elif [ -n "$shell_pid" ]; then
		prev_shell_pid=$shell_pid
		prev_shell_j=$(jiffies "$shell_pid" || echo 0)
		shell_strikes=0
	fi

	if [ "$fip_pid" = "$prev_fip_pid" ]; then
		if j=$(jiffies "$fip_pid"); then
			pct=$(((j - prev_fip_j) * 100 / (CLK * dt)))
			prev_fip_j=$j
			if [ "$pct" -ge "$FIP_TRIP" ]; then
				fip_strikes=$((fip_strikes + 1))
				log "fipindicateur at ${pct}% CPU ($fip_strikes/$STRIKES)"
			else
				fip_strikes=0
			fi
		fi
	else
		prev_fip_pid=$fip_pid
		prev_fip_j=$(jiffies "$fip_pid" || echo 0)
		fip_strikes=0
	fi

	prev_t=$now_t

	if [ "$shell_strikes" -ge "$STRIKES" ]; then
		kill_fip "gnome-shell sature le CPU pendant que fipindicateur tourne"
		shell_strikes=0 fip_strikes=0 ping_strikes=0
	elif [ "$fip_strikes" -ge "$STRIKES" ]; then
		kill_fip "fipindicateur consomme trop de CPU"
		shell_strikes=0 fip_strikes=0 ping_strikes=0
	fi

	sleep "$INTERVAL"
done
