// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/validation"
)

type wifiConnectField int

const (
	wifiConnectSSID wifiConnectField = iota
	wifiConnectSecurity
	wifiConnectPassword
	wifiConnectShowPassword
	wifiConnectAutoconnect
	wifiConnectPriority
	wifiConnectSubmit
	wifiConnectCancel
	wifiConnectFieldCount
)

type wifiConnectAction int

const (
	wifiConnectActionNone wifiConnectAction = iota
	wifiConnectActionSubmit
	wifiConnectActionCancel
)

type wifiSecurityOption struct {
	label   string
	keyMgmt string
}

var hiddenSecurityOptions = []wifiSecurityOption{
	{label: "Open", keyMgmt: ""},
	{label: "WPA/WPA2 Personal", keyMgmt: "wpa-psk"},
	{label: "WPA3 Personal", keyMgmt: "sae"},
	{label: "Enhanced Open (OWE)", keyMgmt: "owe"},
}

type wifiConnectForm struct {
	devicePath      string
	accessPointPath string
	hidden          bool
	field           wifiConnectField
	ssid            textinput.Model
	password        textinput.Model
	priority        textinput.Model
	securityIndex   int
	securityLocked  bool
	securityLabel   string
	keyManagement   string
	autoconnect     bool
	showPassword    bool
	strength        uint8
	message         string
	messageIsError  bool
}

func newVisibleWiFiConnectForm(devicePath string, network model.WiFiNetwork) *wifiConnectForm {
	form := newWiFiConnectFormBase(devicePath)
	form.accessPointPath = network.ObjectPath
	form.ssid.SetValue(network.SSID)
	form.securityLocked = true
	form.securityLabel = string(network.Security)
	form.keyManagement = network.KeyManagement
	form.strength = network.Strength
	form.field = form.firstInteractiveField()
	form.syncFocus()
	return form
}

func newHiddenWiFiConnectForm(devicePath string) *wifiConnectForm {
	form := newWiFiConnectFormBase(devicePath)
	form.hidden = true
	form.securityIndex = 1
	form.field = wifiConnectSSID
	form.syncFocus()
	return form
}

func newWiFiConnectFormBase(devicePath string) *wifiConnectForm {
	ssid := newFormInput("Network name")
	ssid.CharLimit = 32

	password := textinput.New()
	password.Prompt = ""
	password.Placeholder = "Wi-Fi password"
	password.CharLimit = 128
	password.SetWidth(44)
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '•'

	priority := newFormInput("0")
	priority.CharLimit = 12

	return &wifiConnectForm{
		devicePath:  devicePath,
		ssid:        ssid,
		password:    password,
		priority:    priority,
		autoconnect: true,
	}
}

func (f *wifiConnectForm) handleKey(msg tea.KeyPressMsg) (wifiConnectAction, tea.Cmd) {
	f.message = ""
	f.messageIsError = false

	switch msg.String() {
	case "esc":
		return wifiConnectActionCancel, nil
	case "up", "shift+tab":
		return wifiConnectActionNone, f.moveField(-1)
	case "down", "tab":
		return wifiConnectActionNone, f.moveField(1)
	case "enter":
		switch f.field {
		case wifiConnectSubmit:
			return wifiConnectActionSubmit, nil
		case wifiConnectCancel:
			return wifiConnectActionCancel, nil
		default:
			return wifiConnectActionNone, f.moveField(1)
		}
	}

	if input := f.inputForField(f.field); input != nil {
		updated, cmd := input.Update(msg)
		*input = updated
		return wifiConnectActionNone, cmd
	}

	switch f.field {
	case wifiConnectSecurity:
		if !f.securityLocked {
			switch msg.String() {
			case "left":
				f.cycleSecurity(-1)
			case "right", " ":
				f.cycleSecurity(1)
			}
		}
	case wifiConnectShowPassword:
		switch msg.String() {
		case "left", "right", " ":
			f.showPassword = !f.showPassword
			f.syncPasswordEcho()
		}
	case wifiConnectAutoconnect:
		switch msg.String() {
		case "left", "right", " ":
			f.autoconnect = !f.autoconnect
		}
	}

	f.ensureVisibleField()
	f.syncFocus()
	return wifiConnectActionNone, nil
}

func (f *wifiConnectForm) buildRequest() (model.WiFiConnectRequest, error) {
	ssid := strings.TrimSpace(f.ssid.Value())
	if err := validation.ValidateSSID(ssid); err != nil {
		return model.WiFiConnectRequest{}, err
	}

	priority := int64(0)
	if value := strings.TrimSpace(f.priority.Value()); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return model.WiFiConnectRequest{}, fmt.Errorf("priority must be a whole number")
		}
		priority = parsed
	}

	request := model.WiFiConnectRequest{
		DevicePath:          f.devicePath,
		AccessPointPath:     f.accessPointPath,
		SSID:                ssid,
		KeyManagement:       f.currentKeyManagement(),
		Hidden:              f.hidden,
		Autoconnect:         f.autoconnect,
		AutoconnectPriority: int32(priority),
	}
	if f.passwordRequired() {
		request.Password = f.password.Value()
	}
	if err := validation.ValidateWiFiConnectRequest(request); err != nil {
		return model.WiFiConnectRequest{}, err
	}
	return request, nil
}

func (f *wifiConnectForm) clearSecret() {
	f.password.Reset()
	f.password.Blur()
}

func (f *wifiConnectForm) render(width int, connecting bool) string {
	if width < 48 {
		width = 48
	}

	var out strings.Builder
	if f.hidden {
		out.WriteString(titleStyle.Render("Connect to hidden Wi-Fi"))
	} else {
		out.WriteString(titleStyle.Render("Connect to " + emptyFallback(f.ssid.Value(), "Wi-Fi")))
	}
	out.WriteString("\n")
	if !f.hidden {
		out.WriteString(mutedStyle.Render(fmt.Sprintf("%s · %d%% signal", f.securityLabel, f.strength)))
	}
	out.WriteString("\n\n")

	for field := wifiConnectField(0); field < wifiConnectFieldCount; field++ {
		if !f.fieldVisible(field) {
			continue
		}
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
	if connecting {
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Connecting..."))
		out.WriteString("\n")
	}

	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ or Tab move   ←/→ change options   Enter next/select   Esc cancel"))
	return out.String()
}

func (f *wifiConnectForm) renderField(field wifiConnectField, width int) string {
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

func (f *wifiConnectForm) fieldLabelValue(field wifiConnectField) (string, string) {
	switch field {
	case wifiConnectSSID:
		return "Network name", f.ssid.View()
	case wifiConnectSecurity:
		if f.securityLocked {
			return "Security", f.securityLabel
		}
		return "Security", optionValue(hiddenSecurityOptions[f.securityIndex].label)
	case wifiConnectPassword:
		return "Password", f.password.View()
	case wifiConnectShowPassword:
		if f.showPassword {
			return "Show password", optionValue("On")
		}
		return "Show password", optionValue("Off")
	case wifiConnectAutoconnect:
		if f.autoconnect {
			return "Autoconnect", optionValue("On")
		}
		return "Autoconnect", optionValue("Off")
	case wifiConnectPriority:
		return "Priority", f.priority.View()
	case wifiConnectSubmit:
		return "", goodStyle.Render("[ Connect ]")
	case wifiConnectCancel:
		return "", mutedStyle.Render("[ Cancel ]")
	default:
		return "", ""
	}
}

func (f *wifiConnectForm) moveField(direction int) tea.Cmd {
	next := int(f.field)
	for attempts := 0; attempts < int(wifiConnectFieldCount); attempts++ {
		next += direction
		if next < 0 {
			next = int(wifiConnectFieldCount) - 1
		}
		if next >= int(wifiConnectFieldCount) {
			next = 0
		}
		field := wifiConnectField(next)
		if f.fieldVisible(field) && f.fieldInteractive(field) {
			f.field = field
			return f.syncFocus()
		}
	}
	return nil
}

func (f *wifiConnectForm) firstInteractiveField() wifiConnectField {
	for field := wifiConnectField(0); field < wifiConnectFieldCount; field++ {
		if f.fieldVisible(field) && f.fieldInteractive(field) {
			return field
		}
	}
	return wifiConnectSubmit
}

func (f *wifiConnectForm) ensureVisibleField() {
	if f.fieldVisible(f.field) && f.fieldInteractive(f.field) {
		return
	}
	_ = f.moveField(1)
}

func (f *wifiConnectForm) fieldVisible(field wifiConnectField) bool {
	switch field {
	case wifiConnectSSID:
		return f.hidden
	case wifiConnectSecurity:
		return f.hidden
	case wifiConnectPassword, wifiConnectShowPassword:
		return f.passwordRequired()
	default:
		return true
	}
}

func (f *wifiConnectForm) fieldInteractive(field wifiConnectField) bool {
	if field == wifiConnectSecurity && f.securityLocked {
		return false
	}
	return true
}

func (f *wifiConnectForm) inputForField(field wifiConnectField) *textinput.Model {
	switch field {
	case wifiConnectSSID:
		return &f.ssid
	case wifiConnectPassword:
		return &f.password
	case wifiConnectPriority:
		return &f.priority
	default:
		return nil
	}
}

func (f *wifiConnectForm) syncFocus() tea.Cmd {
	f.ssid.Blur()
	f.password.Blur()
	f.priority.Blur()
	input := f.inputForField(f.field)
	if input == nil {
		return nil
	}
	return input.Focus()
}

func (f *wifiConnectForm) cycleSecurity(direction int) {
	f.securityIndex = (f.securityIndex + direction + len(hiddenSecurityOptions)) % len(hiddenSecurityOptions)
	if !f.passwordRequired() {
		f.password.Reset()
		f.showPassword = false
		f.syncPasswordEcho()
	}
}

func (f *wifiConnectForm) currentKeyManagement() string {
	if f.securityLocked {
		return f.keyManagement
	}
	return hiddenSecurityOptions[f.securityIndex].keyMgmt
}

func (f *wifiConnectForm) passwordRequired() bool {
	keyManagement := f.currentKeyManagement()
	return keyManagement == "wpa-psk" || keyManagement == "sae"
}

func (f *wifiConnectForm) syncPasswordEcho() {
	if f.showPassword {
		f.password.EchoMode = textinput.EchoNormal
		return
	}
	f.password.EchoMode = textinput.EchoPassword
}
