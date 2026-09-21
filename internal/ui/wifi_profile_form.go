// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type wifiProfileField int

const (
	wifiProfileAutoconnect wifiProfileField = iota
	wifiProfilePriority
	wifiProfileSave
	wifiProfileCancel
	wifiProfileFieldCount
)

type wifiProfileAction int

const (
	wifiProfileActionNone wifiProfileAction = iota
	wifiProfileActionSave
	wifiProfileActionCancel
)

type wifiProfileForm struct {
	profile        model.ConnectionProfile
	field          wifiProfileField
	autoconnect    bool
	priority       textinput.Model
	message        string
	messageIsError bool
}

func newWiFiProfileForm(profile model.ConnectionProfile) *wifiProfileForm {
	priority := newFormInput("0")
	priority.CharLimit = 12
	priority.SetValue(strconv.FormatInt(int64(profile.AutoconnectPriority), 10))

	form := &wifiProfileForm{
		profile:     profile,
		autoconnect: profile.Autoconnect,
		priority:    priority,
	}
	form.syncFocus()
	return form
}

func (f *wifiProfileForm) handleKey(msg tea.KeyPressMsg) (wifiProfileAction, tea.Cmd) {
	f.message = ""
	f.messageIsError = false

	switch msg.String() {
	case "esc":
		return wifiProfileActionCancel, nil
	case "up", "shift+tab":
		return wifiProfileActionNone, f.moveField(-1)
	case "down", "tab":
		return wifiProfileActionNone, f.moveField(1)
	case "enter":
		switch f.field {
		case wifiProfileSave:
			return wifiProfileActionSave, nil
		case wifiProfileCancel:
			return wifiProfileActionCancel, nil
		default:
			return wifiProfileActionNone, f.moveField(1)
		}
	}

	if f.field == wifiProfilePriority {
		updated, cmd := f.priority.Update(msg)
		f.priority = updated
		return wifiProfileActionNone, cmd
	}

	if f.field == wifiProfileAutoconnect {
		switch msg.String() {
		case "left", "right", " ":
			f.autoconnect = !f.autoconnect
		}
	}
	return wifiProfileActionNone, nil
}

func (f *wifiProfileForm) buildUpdate() (model.WiFiProfileUpdate, error) {
	priority := int64(0)
	if value := strings.TrimSpace(f.priority.Value()); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return model.WiFiProfileUpdate{}, fmt.Errorf("priority must be a whole number")
		}
		priority = parsed
	}

	return model.WiFiProfileUpdate{
		ProfilePath:         f.profile.ObjectPath,
		Autoconnect:         f.autoconnect,
		AutoconnectPriority: int32(priority),
	}, nil
}

func (f *wifiProfileForm) render(width int, saving bool) string {
	if width < 48 {
		width = 48
	}

	var out strings.Builder
	out.WriteString(titleStyle.Render("Saved Wi-Fi profile · " + emptyFallback(f.profile.SSID, f.profile.ID)))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render("Password: unchanged"))
	out.WriteString("\n\n")

	for field := wifiProfileField(0); field < wifiProfileFieldCount; field++ {
		out.WriteString(f.renderField(field, width))
		out.WriteString("\n")
	}

	if f.message != "" {
		out.WriteString("\n")
		if f.messageIsError {
			out.WriteString(errorStyle.Render(f.message))
		} else {
			out.WriteString(goodStyle.Render(f.message))
		}
		out.WriteString("\n")
	}
	if saving {
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Saving profile..."))
		out.WriteString("\n")
	}

	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ or Tab move   ←/→ change option   Enter next/select   Esc cancel"))
	return out.String()
}

func (f *wifiProfileForm) renderField(field wifiProfileField, width int) string {
	label, value := f.fieldLabelValue(field)
	marker := "  "
	style := cardStyle
	if field == f.field {
		marker = "› "
		style = selectedCardStyle
	}
	line := fmt.Sprintf("%s%-18s %s", marker, label, value)
	return style.Width(cardContentWidth(width)).Render(line)
}

func (f *wifiProfileForm) fieldLabelValue(field wifiProfileField) (string, string) {
	switch field {
	case wifiProfileAutoconnect:
		if f.autoconnect {
			return "Autoconnect", optionValue("On")
		}
		return "Autoconnect", optionValue("Off")
	case wifiProfilePriority:
		return "Priority", f.priority.View()
	case wifiProfileSave:
		return "", goodStyle.Render("[ Save ]")
	case wifiProfileCancel:
		return "", mutedStyle.Render("[ Cancel ]")
	default:
		return "", ""
	}
}

func (f *wifiProfileForm) moveField(direction int) tea.Cmd {
	next := int(f.field) + direction
	if next < 0 {
		next = int(wifiProfileFieldCount) - 1
	}
	if next >= int(wifiProfileFieldCount) {
		next = 0
	}
	f.field = wifiProfileField(next)
	return f.syncFocus()
}

func (f *wifiProfileForm) syncFocus() tea.Cmd {
	f.priority.Blur()
	if f.field == wifiProfilePriority {
		return f.priority.Focus()
	}
	return nil
}
