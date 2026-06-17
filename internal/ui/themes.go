package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// ocPalette is a theme's colors in the OpenCode model for one mode.
type ocPalette struct {
	bg, fg, primary, accent, good, warn, bad, info string
}

// ocTheme is a built-in theme with a light and dark variant; buildTheme picks
// the variant matching the detected terminal background.
type ocTheme struct {
	id, title   string
	light, dark ocPalette
}

// ocToPalette maps the OpenCode colors onto ku's Palette, deriving the muted,
// border, and selection-background tones the source model doesn't carry.
func ocToPalette(p ocPalette) Palette {
	accent2 := firstHex(p.accent, p.info, p.primary)
	return Palette{
		Accent:     lipgloss.Color(p.primary),
		Accent2:    lipgloss.Color(accent2),
		Fg:         lipgloss.Color(p.fg),
		Muted:      lipgloss.Color(mixHex(p.fg, p.bg, 0.45)),
		Border:     lipgloss.Color(mixHex(p.fg, p.bg, 0.72)),
		Good:       lipgloss.Color(p.good),
		Warn:       lipgloss.Color(p.warn),
		Bad:        lipgloss.Color(p.bad),
		SelFg:      lipgloss.Color(p.fg),
		SelBg:      lipgloss.Color(mixHex(p.bg, p.primary, 0.20)),
		HeaderBg:   lipgloss.Color(p.primary),
		LogoFg:     lipgloss.Color(p.bg),
		ReverseSel: false,
	}
}

func firstHex(opts ...string) string {
	for _, s := range opts {
		if s != "" {
			return s
		}
	}
	return "#000000"
}

func parseHex(s string) (r, g, b uint8) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = s[0:1] + s[0:1] + s[1:2] + s[1:2] + s[2:3] + s[2:3]
	}
	if len(s) != 6 {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// mixHex blends a toward b by t in [0,1] and returns "#rrggbb".
func mixHex(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}

var ocByID = func() map[string]ocTheme {
	m := make(map[string]ocTheme, len(ocThemeList))
	for _, t := range ocThemeList {
		m[t.id] = t
	}
	return m
}()

// ocThemeList holds the built-in themes in picker display order.
var ocThemeList = []ocTheme{
	{id: "amoled", title: "AMOLED",
		light: ocPalette{bg: "#f0f0f0", fg: "#0a0a0a", primary: "#6200ff", accent: "#ff0080", good: "#00e676", warn: "#ffab00", bad: "#ff1744", info: "#00b0ff"},
		dark:  ocPalette{bg: "#000000", fg: "#ffffff", primary: "#b388ff", accent: "#ff4081", good: "#00ff88", warn: "#ffea00", bad: "#ff1744", info: "#18ffff"}},
	{id: "aura", title: "Aura",
		light: ocPalette{bg: "#f5f0ff", fg: "#2d2640", primary: "#a277ff", accent: "#d94f4f", good: "#40bf7a", warn: "#d9a24a", bad: "#d94f4f", info: "#5bb8d9"},
		dark:  ocPalette{bg: "#15141b", fg: "#edecee", primary: "#a277ff", accent: "#ff6767", good: "#61ffca", warn: "#ffca85", bad: "#ff6767", info: "#82e2ff"}},
	{id: "ayu", title: "Ayu",
		light: ocPalette{bg: "#fdfaf4", fg: "#4f5964", primary: "#4aa8c8", accent: "#ef7d71", good: "#5fb978", warn: "#ea9f41", bad: "#e6656a", info: "#2f9bce"},
		dark:  ocPalette{bg: "#0f1419", fg: "#d6dae0", primary: "#3fb7e3", accent: "#f2856f", good: "#78d05c", warn: "#e4a75c", bad: "#f58572", info: "#66c6f1"}},
	{id: "carbonfox", title: "Carbonfox",
		light: ocPalette{bg: "#8e8e8e", fg: "#161616", primary: "#0072c3", accent: "#da1e28", good: "#198038", warn: "#f1c21b", bad: "#da1e28", info: "#0043ce"},
		dark:  ocPalette{bg: "#393939", fg: "#f2f4f8", primary: "#33b1ff", accent: "#ff8389", good: "#42be65", warn: "#f1c21b", bad: "#ff8389", info: "#78a9ff"}},
	{id: "catppuccin-frappe", title: "Catppuccin Frappe",
		light: ocPalette{bg: "#303446", fg: "#c6d0f5", primary: "#8da4e2", accent: "#f4b8e4", good: "#a6d189", warn: "#e5c890", bad: "#e78284", info: "#81c8be"},
		dark:  ocPalette{bg: "#303446", fg: "#c6d0f5", primary: "#8da4e2", accent: "#f4b8e4", good: "#a6d189", warn: "#e5c890", bad: "#e78284", info: "#81c8be"}},
	{id: "catppuccin-macchiato", title: "Catppuccin Macchiato",
		light: ocPalette{bg: "#24273a", fg: "#cad3f5", primary: "#8aadf4", accent: "#f5bde6", good: "#a6da95", warn: "#eed49f", bad: "#ed8796", info: "#8bd5ca"},
		dark:  ocPalette{bg: "#24273a", fg: "#cad3f5", primary: "#8aadf4", accent: "#f5bde6", good: "#a6da95", warn: "#eed49f", bad: "#ed8796", info: "#8bd5ca"}},
	{id: "catppuccin", title: "Catppuccin",
		light: ocPalette{bg: "#f5e0dc", fg: "#4c4f69", primary: "#7287fd", accent: "#d20f39", good: "#40a02b", warn: "#df8e1d", bad: "#d20f39", info: "#04a5e5"},
		dark:  ocPalette{bg: "#1e1e2e", fg: "#cdd6f4", primary: "#b4befe", accent: "#f38ba8", good: "#a6d189", warn: "#f4b8e4", bad: "#f38ba8", info: "#89dceb"}},
	{id: "cobalt2", title: "Cobalt2",
		light: ocPalette{bg: "#ffffff", fg: "#193549", primary: "#0066cc", accent: "#00acc1", good: "#4caf50", warn: "#ff9800", bad: "#e91e63", info: "#ff5722"},
		dark:  ocPalette{bg: "#193549", fg: "#ffffff", primary: "#0088ff", accent: "#2affdf", good: "#9eff80", warn: "#ffc600", bad: "#ff0088", info: "#ff9d00"}},
	{id: "cursor", title: "Cursor",
		light: ocPalette{bg: "#fcfcfc", fg: "#141414", primary: "#6f9ba6", accent: "#6f9ba6", good: "#1f8a65", warn: "#db704b", bad: "#cf2d56", info: "#3c7cab"},
		dark:  ocPalette{bg: "#181818", fg: "#e4e4e4", primary: "#88c0d0", accent: "#88c0d0", good: "#3fa266", warn: "#f1b467", bad: "#e34671", info: "#81a1c1"}},
	{id: "dracula", title: "Dracula",
		light: ocPalette{bg: "#f8f8f2", fg: "#1f1f2f", primary: "#7c6bf5", accent: "#d16090", good: "#2fbf71", warn: "#f7a14d", bad: "#d9536f", info: "#1d7fc5"},
		dark:  ocPalette{bg: "#1d1e28", fg: "#f8f8f2", primary: "#bd93f9", accent: "#ff79c6", good: "#50fa7b", warn: "#ffb86c", bad: "#ff5555", info: "#8be9fd"}},
	{id: "everforest", title: "Everforest",
		light: ocPalette{bg: "#fdf6e3", fg: "#5c6a72", primary: "#8da101", accent: "#df69ba", good: "#8da101", warn: "#f57d26", bad: "#f85552", info: "#35a77c"},
		dark:  ocPalette{bg: "#2d353b", fg: "#d3c6aa", primary: "#a7c080", accent: "#d699b6", good: "#a7c080", warn: "#e69875", bad: "#e67e80", info: "#83c092"}},
	{id: "flexoki", title: "Flexoki",
		light: ocPalette{bg: "#fffcf0", fg: "#100f0f", primary: "#205ea6", accent: "#bc5215", good: "#66800b", warn: "#bc5215", bad: "#af3029", info: "#24837b"},
		dark:  ocPalette{bg: "#100f0f", fg: "#cecdc3", primary: "#da702c", accent: "#8b7ec8", good: "#879a39", warn: "#da702c", bad: "#d14d41", info: "#3aa99f"}},
	{id: "github", title: "GitHub",
		light: ocPalette{bg: "#ffffff", fg: "#24292f", primary: "#0969da", accent: "#1b7c83", good: "#1a7f37", warn: "#9a6700", bad: "#cf222e", info: "#bc4c00"},
		dark:  ocPalette{bg: "#0d1117", fg: "#c9d1d9", primary: "#58a6ff", accent: "#39c5cf", good: "#3fb950", warn: "#e3b341", bad: "#f85149", info: "#d29922"}},
	{id: "gruvbox", title: "Gruvbox",
		light: ocPalette{bg: "#fbf1c7", fg: "#3c3836", primary: "#076678", accent: "#9d0006", good: "#79740e", warn: "#b57614", bad: "#9d0006", info: "#8f3f71"},
		dark:  ocPalette{bg: "#282828", fg: "#ebdbb2", primary: "#83a598", accent: "#fb4934", good: "#b8bb26", warn: "#fabd2f", bad: "#fb4934", info: "#d3869b"}},
	{id: "kanagawa", title: "Kanagawa",
		light: ocPalette{bg: "#f2e9de", fg: "#54433a", primary: "#2d4f67", accent: "#d27e99", good: "#98bb6c", warn: "#d7a657", bad: "#e82424", info: "#76946a"},
		dark:  ocPalette{bg: "#1f1f28", fg: "#dcd7ba", primary: "#7e9cd8", accent: "#d27e99", good: "#98bb6c", warn: "#d7a657", bad: "#e82424", info: "#76946a"}},
	{id: "lucent-orng", title: "Lucent Orng",
		light: ocPalette{bg: "#fff5f0", fg: "#1a1a1a", primary: "#ec5b2b", accent: "#c94d24", good: "#0062d1", warn: "#ec5b2b", bad: "#d1383d", info: "#318795"},
		dark:  ocPalette{bg: "#2a1a15", fg: "#eeeeee", primary: "#ec5b2b", accent: "#fff7f1", good: "#6ba1e6", warn: "#ec5b2b", bad: "#e06c75", info: "#56b6c2"}},
	{id: "material", title: "Material",
		light: ocPalette{bg: "#fafafa", fg: "#263238", primary: "#6182b8", accent: "#39adb5", good: "#91b859", warn: "#ffb300", bad: "#e53935", info: "#f4511e"},
		dark:  ocPalette{bg: "#263238", fg: "#eeffff", primary: "#82aaff", accent: "#89ddff", good: "#c3e88d", warn: "#ffcb6b", bad: "#f07178", info: "#ffcb6b"}},
	{id: "matrix", title: "Matrix",
		light: ocPalette{bg: "#eef3ea", fg: "#203022", primary: "#1cc24b", accent: "#c770ff", good: "#1cc24b", warn: "#e6ff57", bad: "#ff4b4b", info: "#30b3ff"},
		dark:  ocPalette{bg: "#0a0e0a", fg: "#62ff94", primary: "#2eff6a", accent: "#c770ff", good: "#62ff94", warn: "#e6ff57", bad: "#ff4b4b", info: "#30b3ff"}},
	{id: "mercury", title: "Mercury",
		light: ocPalette{bg: "#ffffff", fg: "#363644", primary: "#5266eb", accent: "#8da4f5", good: "#036e43", warn: "#a44200", bad: "#b0175f", info: "#007f95"},
		dark:  ocPalette{bg: "#171721", fg: "#dddde5", primary: "#8da4f5", accent: "#8da4f5", good: "#77c599", warn: "#fc9b6f", bad: "#fc92b4", info: "#77becf"}},
	{id: "monokai", title: "Monokai",
		light: ocPalette{bg: "#fdf8ec", fg: "#292318", primary: "#bf7bff", accent: "#d9487c", good: "#4fb54b", warn: "#f1a948", bad: "#e54b4b", info: "#2d9ad7"},
		dark:  ocPalette{bg: "#272822", fg: "#f8f8f2", primary: "#ae81ff", accent: "#f92672", good: "#a6e22e", warn: "#fd971f", bad: "#f92672", info: "#66d9ef"}},
	{id: "nightowl", title: "Night Owl",
		light: ocPalette{bg: "#f0f0f0", fg: "#403f53", primary: "#4876d6", accent: "#aa0982", good: "#2aa298", warn: "#c96765", bad: "#de3d3b", info: "#4876d6"},
		dark:  ocPalette{bg: "#011627", fg: "#d6deeb", primary: "#82aaff", accent: "#f78c6c", good: "#c5e478", warn: "#ecc48d", bad: "#ef5350", info: "#82aaff"}},
	{id: "nord", title: "Nord",
		light: ocPalette{bg: "#eceff4", fg: "#2e3440", primary: "#5e81ac", accent: "#bf616a", good: "#8fbcbb", warn: "#d08770", bad: "#bf616a", info: "#81a1c1"},
		dark:  ocPalette{bg: "#2e3440", fg: "#e5e9f0", primary: "#88c0d0", accent: "#d57780", good: "#a3be8c", warn: "#d08770", bad: "#bf616a", info: "#81a1c1"}},
	{id: "oc-2", title: "OC-2",
		light: ocPalette{bg: "#f7f7f7", fg: "#171311", primary: "#dcde8d", good: "#12c905", warn: "#ffdc17", bad: "#fc533a", info: "#a753ae"},
		dark:  ocPalette{bg: "#1f1f1f", fg: "#f1ece8", primary: "#fab283", good: "#12c905", warn: "#fcd53a", bad: "#fc533a", info: "#edb2f1"}},
	{id: "one-dark", title: "One Dark",
		light: ocPalette{bg: "#fafafa", fg: "#383a42", primary: "#4078f2", accent: "#0184bc", good: "#50a14f", warn: "#c18401", bad: "#e45649", info: "#986801"},
		dark:  ocPalette{bg: "#282c34", fg: "#abb2bf", primary: "#61afef", accent: "#56b6c2", good: "#98c379", warn: "#e5c07b", bad: "#e06c75", info: "#d19a66"}},
	{id: "onedarkpro", title: "One Dark Pro",
		light: ocPalette{bg: "#f5f6f8", fg: "#2b303b", primary: "#528bff", accent: "#d85462", good: "#4fa66d", warn: "#d19a66", bad: "#e06c75", info: "#61afef"},
		dark:  ocPalette{bg: "#1e222a", fg: "#abb2bf", primary: "#61afef", accent: "#e06c75", good: "#98c379", warn: "#e5c07b", bad: "#e06c75", info: "#56b6c2"}},
	{id: "opencode", title: "OpenCode",
		light: ocPalette{bg: "#ffffff", fg: "#1a1a1a", primary: "#3b7dd8", accent: "#d68c27", good: "#3d9a57", warn: "#d68c27", bad: "#d1383d", info: "#318795"},
		dark:  ocPalette{bg: "#0a0a0a", fg: "#eeeeee", primary: "#fab283", accent: "#9d7cd8", good: "#7fd88f", warn: "#f5a742", bad: "#e06c75", info: "#56b6c2"}},
	{id: "orng", title: "Orng",
		light: ocPalette{bg: "#ffffff", fg: "#1a1a1a", primary: "#ec5b2b", accent: "#c94d24", good: "#0062d1", warn: "#ec5b2b", bad: "#d1383d", info: "#318795"},
		dark:  ocPalette{bg: "#0a0a0a", fg: "#eeeeee", primary: "#ec5b2b", accent: "#fff7f1", good: "#6ba1e6", warn: "#ec5b2b", bad: "#e06c75", info: "#56b6c2"}},
	{id: "osaka-jade", title: "Osaka Jade",
		light: ocPalette{bg: "#f6f5dd", fg: "#111c18", primary: "#1faa90", accent: "#3d7a52", good: "#3d7a52", warn: "#b5a020", bad: "#c7392d", info: "#1faa90"},
		dark:  ocPalette{bg: "#111c18", fg: "#c1c497", primary: "#2dd5b7", accent: "#549e6a", good: "#549e6a", warn: "#e5c736", bad: "#ff5345", info: "#2dd5b7"}},
	{id: "palenight", title: "Palenight",
		light: ocPalette{bg: "#fafafa", fg: "#292d3e", primary: "#4976eb", accent: "#00acc1", good: "#91b859", warn: "#ffb300", bad: "#e53935", info: "#f4511e"},
		dark:  ocPalette{bg: "#292d3e", fg: "#a6accd", primary: "#82aaff", accent: "#89ddff", good: "#c3e88d", warn: "#ffcb6b", bad: "#f07178", info: "#f78c6c"}},
	{id: "rosepine", title: "Rose Pine",
		light: ocPalette{bg: "#faf4ed", fg: "#575279", primary: "#31748f", accent: "#d7827e", good: "#286983", warn: "#ea9d34", bad: "#b4637a", info: "#56949f"},
		dark:  ocPalette{bg: "#191724", fg: "#e0def4", primary: "#9ccfd8", accent: "#ebbcba", good: "#31748f", warn: "#f6c177", bad: "#eb6f92", info: "#9ccfd8"}},
	{id: "shadesofpurple", title: "Shades of Purple",
		light: ocPalette{bg: "#f7ebff", fg: "#3b2c59", primary: "#7a5af8", accent: "#ff6bd5", good: "#3dd598", warn: "#f7c948", bad: "#ff6bd5", info: "#62d4ff"},
		dark:  ocPalette{bg: "#1a102b", fg: "#f5f0ff", primary: "#c792ff", accent: "#ff7ac6", good: "#7be0b0", warn: "#ffd580", bad: "#ff7ac6", info: "#7dd4ff"}},
	{id: "solarized", title: "Solarized",
		light: ocPalette{bg: "#fdf6e3", fg: "#586e75", primary: "#268bd2", accent: "#d33682", good: "#859900", warn: "#b58900", bad: "#dc322f", info: "#2aa198"},
		dark:  ocPalette{bg: "#002b36", fg: "#93a1a1", primary: "#6c71c4", accent: "#d33682", good: "#859900", warn: "#b58900", bad: "#dc322f", info: "#2aa198"}},
	{id: "synthwave84", title: "Synthwave '84",
		light: ocPalette{bg: "#fafafa", fg: "#262335", primary: "#00bcd4", accent: "#9c27b0", good: "#4caf50", warn: "#ff9800", bad: "#f44336", info: "#ff5722"},
		dark:  ocPalette{bg: "#262335", fg: "#ffffff", primary: "#36f9f6", accent: "#b084eb", good: "#72f1b8", warn: "#fede5d", bad: "#fe4450", info: "#ff8b39"}},
	{id: "tokyonight", title: "Tokyo Night",
		light: ocPalette{bg: "#e1e2e7", fg: "#273153", primary: "#2e7de9", accent: "#b15c00", good: "#587539", warn: "#8c6c3e", bad: "#c94060", info: "#007197"},
		dark:  ocPalette{bg: "#1a1b26", fg: "#c0caf5", primary: "#7aa2f7", accent: "#ff9e64", good: "#9ece6a", warn: "#e0af68", bad: "#f7768e", info: "#7dcfff"}},
	{id: "vercel", title: "Vercel",
		light: ocPalette{bg: "#ffffff", fg: "#171717", primary: "#0070f3", accent: "#8e4ec6", good: "#388e3c", warn: "#ff9500", bad: "#dc3545", info: "#0070f3"},
		dark:  ocPalette{bg: "#000000", fg: "#ededed", primary: "#0070f3", accent: "#8e4ec6", good: "#46a758", warn: "#ffb224", bad: "#e5484d", info: "#52a8ff"}},
	{id: "vesper", title: "Vesper",
		light: ocPalette{bg: "#f0f0f0", fg: "#101010", primary: "#ffc799", accent: "#b30000", good: "#99ffe4", warn: "#ffc799", bad: "#ff8080", info: "#ffc799"},
		dark:  ocPalette{bg: "#101010", fg: "#ffffff", primary: "#ffc799", accent: "#ff8080", good: "#99ffe4", warn: "#ffc799", bad: "#ff8080", info: "#ffc799"}},
	{id: "zenburn", title: "Zenburn",
		light: ocPalette{bg: "#ffffef", fg: "#3f3f3f", primary: "#5f7f8f", accent: "#5f8f8f", good: "#5f8f5f", warn: "#8f8f5f", bad: "#8f5f5f", info: "#8f7f5f"},
		dark:  ocPalette{bg: "#3f3f3f", fg: "#dcdccc", primary: "#8cd0d3", accent: "#93e0e3", good: "#7f9f7f", warn: "#f0dfaf", bad: "#cc9393", info: "#dfaf8f"}},
}
