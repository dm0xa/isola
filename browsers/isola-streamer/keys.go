package main

import "strings"

// translateChord normalizes chord key names (e.g. "Enter" -> "Return").
func translateChord(chord string) string {
	parts := strings.Split(chord, "+")
	for i, p := range parts {
		switch p {
		case "ctrl", "shift", "alt", "meta", "super", "hyper":
			continue
		}
		if mapped := mapKey(p); mapped != "" {
			parts[i] = mapped
		}
	}
	return strings.Join(parts, "+")
}

// mapKey translates UI key names to CDP/generic key names.
func mapKey(k string) string {
	switch k {
	case "Enter":
		return "Return"
	case "Escape":
		return "Escape"
	case "Tab":
		return "Tab"
	case "Backspace":
		return "BackSpace"
	case "Delete":
		return "Delete"
	case "Insert":
		return "Insert"
	case "Home":
		return "Home"
	case "End":
		return "End"
	case "PageUp":
		return "PageUp"
	case "PageDown":
		return "PageDown"
	case "ArrowUp":
		return "Up"
	case "ArrowDown":
		return "Down"
	case "ArrowLeft":
		return "Left"
	case "ArrowRight":
		return "Right"
	case "CapsLock":
		return "Caps_Lock"
	case "NumLock":
		return "Num_Lock"
	case "ScrollLock":
		return "Scroll_Lock"
	case "Shift":
		return "Shift_L"
	case "Control":
		return "Control_L"
	case "Alt":
		return "Alt_L"
	case "Meta":
		return ""
	case "AltGraph":
		return "Alt_R"
	case "OS":
		return ""
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		return k
	case "NumpadDivide":
		return "KP_Divide"
	case "NumpadMultiply":
		return "KP_Multiply"
	case "NumpadSubtract":
		return "KP_Subtract"
	case "NumpadAdd":
		return "KP_Add"
	case "NumpadEnter":
		return "KP_Enter"
	case "NumpadDecimal":
		return "KP_Decimal"
	case "Numpad0", "Numpad1", "Numpad2", "Numpad3", "Numpad4", "Numpad5", "Numpad6", "Numpad7", "Numpad8", "Numpad9":
		return "KP_" + k[6:]
	case "PrintScreen":
		return "Print"
	case "Pause":
		return "Pause"
	case " ":
		return "space"
	case "ContextMenu":
		return "Menu"
	case "AudioVolumeMute":
		return "XF86AudioMute"
	case "AudioVolumeDown":
		return "XF86AudioLowerVolume"
	case "AudioVolumeUp":
		return "XF86AudioRaiseVolume"
	case "MediaPlayPause":
		return "XF86AudioPlay"
	case "MediaTrackNext":
		return "XF86AudioNext"
	case "MediaTrackPrevious":
		return "XF86AudioPrev"
	case "MediaStop":
		return "XF86AudioStop"
	case "BrowserSearch":
		return "XF86Search"
	case "BrowserHome":
		return "XF86HomePage"
	case "BrowserBack":
		return "XF86Back"
	case "BrowserForward":
		return "XF86Forward"
	case "BrowserRefresh":
		return "XF86Refresh"
	case "Clear":
		return "Clear"
	}
	if mapped, ok := asciiPunctKeysyms[k]; ok {
		return mapped
	}
	if len(k) == 1 {
		return k
	}
	return ""
}

// asciiPunctKeysyms maps ASCII punctuation to CDP code names.
var asciiPunctKeysyms = map[string]string{
	"!":  "exclam",
	"\"": "quotedbl",
	"#":  "numbersign",
	"$":  "dollar",
	"%":  "percent",
	"&":  "ampersand",
	"'":  "apostrophe",
	"(":  "parenleft",
	")":  "parenright",
	"*":  "asterisk",
	"+":  "plus",
	",":  "comma",
	"-":  "minus",
	".":  "period",
	"/":  "slash",
	":":  "colon",
	";":  "semicolon",
	"<":  "less",
	"=":  "equal",
	">":  "greater",
	"?":  "question",
	"@":  "at",
	"[":  "bracketleft",
	"\\": "backslash",
	"]":  "bracketright",
	"^":  "asciicircum",
	"_":  "underscore",
	"`":  "grave",
	"{":  "braceleft",
	"|":  "bar",
	"}":  "braceright",
	"~":  "asciitilde",
}
