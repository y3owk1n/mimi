# Windows a layout should leave where they are, as jq definitions the layout
# scripts include:
#
#   jq -L "$here" 'include "float-rules"; tileable | ...'
#
# Copy, edit, own. Add bundle identifiers and title patterns to taste.

# floating: true for a window the layout leaves alone.
def floating:
  (.bundleId | IN(
    "com.apple.systempreferences",
    "com.apple.finder",
    "com.apple.ActivityMonitor",
    "com.1password.1password"
  ))
  or (.title | test("^(Preferences|Settings)$"))
  or (.frame.width < 400 and .frame.height < 300);

# tileable: the layout input without its floating windows, with "focused"
# still pointing at the same window, or -1 when that window was dropped.
def tileable:
  (if .focused >= 0 then .windows[.focused].number else null end) as $focusedNumber
  | .windows |= map(select(floating | not))
  | .focused = ((.windows | map(.number) | index($focusedNumber)) // -1);

# display_for: the display whose frame holds the focused window's center, or
# the first display.
def display_for:
  def center: {x: (.x + .width / 2), y: (.y + .height / 2)};
  def contains($p): ($p.x >= .x and $p.x < .x + .width and $p.y >= .y and $p.y < .y + .height);
  (if .focused >= 0 then .windows[.focused].frame | center else null end) as $c
  | ([.displays[] | select($c != null and (.frame | contains($c)))][0] // .displays[0]);
