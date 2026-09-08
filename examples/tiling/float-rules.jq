# Windows a layout should leave where they are. Reads the output of
# "mimi query windows" and prints it back without them, keeping "focused"
# pointing at the same window (or -1 when it was dropped).
#
# Copy, edit, own. Add bundle identifiers and title patterns to taste.

def floating:
  (.bundleId | IN(
    "com.apple.systempreferences",
    "com.apple.finder",
    "com.apple.ActivityMonitor",
    "com.1password.1password"
  ))
  or (.title | test("^(Preferences|Settings)$"))
  or (.frame.width < 400 and .frame.height < 300);

(if .focused >= 0 then .windows[.focused].number else null end) as $focusedNumber
| .windows |= map(select(floating | not))
| .focused = ((.windows | map(.number) | index($focusedNumber)) // -1)
