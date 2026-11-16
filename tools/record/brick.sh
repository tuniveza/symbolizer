#!/bin/bash
# Make it (from the repo): tools/record/brick.sh, then ffmpeg -i $REC_DIR/frames.mp4 -vf "setpts=PTS/1.5,fps=10,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=none" -t 27 docs/media/brick.gif
# Records docs/media/brick.gif: sy brick mode on a hidden Xvfb display. No sound, no real pointer, own processes only.
# Any argument: dry run (screenshots only, no recording).
R=${REC_DIR:-/tmp/sy-record-brick}; mkdir -p $R
D=:98
W=1200; H=700
DRY=$1
cd $R
rm -rf home demo.brick frames.mp4 s*.png
mkdir -p home/work
Xvfb $D -screen 0 ${W}x${H}x24 >/dev/null 2>&1 &
XPID=$!
sleep 2
setxkbmap -display $D gb
env -i HOME=$R/home PATH=$PATH SHELL=/bin/sh PS1='$ ' ENV= XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR DISPLAY=$D KITTY_DISABLE_WAYLAND=1 LANG=en_GB.UTF-8 \
  kitty --config ~/.config/kitty/kitty.conf -o remember_window_size=no -o background_opacity=1 -o font_size=15 \
  -o tab_bar_style=hidden -o allow_remote_control=yes -o initial_window_width=$W -o initial_window_height=$H \
  --class sylivetest \
  sh -c "cd $R/home/work && SY_3D_SOUND=0 SY_NO_NETWORK=1 HYPRLAND_INSTANCE_SIGNATURE= exec /home/tuniveza/an/ca/sy/sy demo.brick" \
  >/dev/null 2>&1 &
KPID=$!
sleep 7
k() { DISPLAY=$D xdotool key --delay 90 "$@"; }
t() { DISPLAY=$D xdotool type --delay 75 "$1"; }
shot() { DISPLAY=$D import -window root $R/$1.png; }
if [ -z "$DRY" ]; then
  ffmpeg -loglevel error -y -f x11grab -framerate 12 -video_size ${W}x${H} -i $D -c:v libx264 -preset ultrafast -crf 18 frames.mp4 &
  FPID=$!
  sleep 1
fi
k Alt_L; sleep 0.15; k Alt_L; sleep 0.25; k b; sleep 4.5          # Alt, Alt, B: brick mode; the starter guide
shot s1
t "write hello in red"; sleep 2.5
k Return; t "shout welcome"; sleep 5                                 # no action for shout: the brain invents one
shot s2
k Return; t "splash bye in blue"; sleep 9
shot s3
k alt+r; sleep 24                                                    # the brain fixes both lines, builds, runs
shot s4
sleep 3
shot s5
if [ -z "$DRY" ]; then kill -INT $FPID; wait $FPID 2>/dev/null; fi
kill $KPID 2>/dev/null; sleep 1; kill $XPID 2>/dev/null
cat $R/home/work/demo.brick; echo "--- the program as it ended"
