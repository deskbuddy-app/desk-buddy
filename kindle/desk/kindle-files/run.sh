#!/bin/sh
# Desk Buddy: hand the screen to the desk app, then give it back to the Kindle.
# Started by "Desk Buddy.sh" in the documents folder. Log: /mnt/us/desk/desk.log
DIR=/mnt/us/desk
LOG=$DIR/desk.log

# Needs root (touchscreen, Kindle UI). The library starts scripts as a normal user.
if [ "$(id -u)" -ne 0 ]; then
  exec /var/local/mkk/su -c "/bin/sh $DIR/run.sh"
fi

# Only one copy at a time.
if [ -f /tmp/desk.pid ] && kill -0 "$(cat /tmp/desk.pid)" 2>/dev/null; then exit 0; fi
echo $$ > /tmp/desk.pid

restore() {
  killall -CONT cvm kppMainApp mesquite webreader 2>/dev/null            # wake the Kindle UI
  lipc-set-prop com.lab126.pillow disableEnablePillow enable 2>/dev/null # top bar back
  lipc-set-prop com.lab126.powerd preventScreenSaver 0 2>/dev/null
  # Make the Kindle redraw its own screen: open Settings, then go Home.
  lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.settings 2>/dev/null
  sleep 2
  lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.home 2>/dev/null
  rm -f /tmp/desk.pid
  echo "== restored $(date)" >> "$LOG"
}
trap restore EXIT
trap 'exit 1' INT TERM HUP

{
  echo "== start $(date)"
  # Keep the log small.
  [ "$(wc -c < "$LOG" 2>/dev/null || echo 0)" -gt 200000 ] && tail -c 50000 "$LOG" > "$LOG.old" && mv "$LOG.old" "$LOG"
} >> "$LOG" 2>&1

# Let the launcher's own pop-up finish before freezing the Kindle UI.
sleep 3

lipc-set-prop com.lab126.powerd preventScreenSaver 1      # no screensaver or sleep
lipc-set-prop com.lab126.pillow disableEnablePillow disable # hide the top bar
killall -STOP cvm kppMainApp mesquite webreader 2>/dev/null # pause the Kindle UI so it can't draw over us

# Run from /tmp so it works whether or not /mnt/us allows running programs.
# Exit code 3: the app has installed an update over Wi-Fi (into $DIR/desk, old one kept
# as desk.old): start again with the new one. (Older builds left it in desk.new.)
while :; do
  cp "$DIR/desk" /tmp/desk-bin && chmod +x /tmp/desk-bin
  /tmp/desk-bin -config "$DIR/config.json" >> "$LOG" 2>&1
  code=$?
  echo "== app exited ($code) $(date)" >> "$LOG"
  if [ "$code" -eq 3 ]; then
    if [ -s "$DIR/desk.new" ]; then
      mv -f "$DIR/desk" "$DIR/desk.old"
      mv -f "$DIR/desk.new" "$DIR/desk"
    fi
    sync
    echo "== updated, starting the new version" >> "$LOG"
    continue
  fi
  break
done
