// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/validation"
)

type editorField int

const (
	fieldIPv4Method editorField = iota
	fieldIPv4Addresses
	fieldIPv4Gateway
	fieldIPv4DNS
	fieldIPv6Method
	fieldIPv6Addresses
	fieldIPv6Gateway
	fieldIPv6DNS
	fieldAutoconnect
	fieldMTU
	editorFieldCount
)

type editorAction int

const (
	editorActionNone editorAction = iota
	editorActionSave
	editorActionCancel
)

type ethernetEditor struct {
	profile        model.EthernetProfile
	field          editorField
	ipv4Method     model.IPMethod
	ipv4Addresses  string
	ipv4Gateway    string
	ipv4DNS        string
	ipv6Method     model.IPMethod
	ipv6Addresses  string
	ipv6Gateway    string
	ipv6DNS        string
	autoconnect    bool
	mtu            string
	textCursor     int
	message        string
	messageIsError bool
}

func newEthernetEditor(profile model.EthernetProfile) *ethernetEditor {
	editor := &ethernetEditor{
		profile:       profile,
		ipv4Method:    profile.IPv4.Method,
		ipv4Addresses: formatProfileAddresses(profile.IPv4.Addresses),
		ipv4Gateway:   profile.IPv4.Gateway,
		ipv4DNS:       strings.Join(profile.IPv4.DNS, ", "),
		ipv6Method:    profile.IPv6.Method,
		ipv6Addresses: formatProfileAddresses(profile.IPv6.Addresses),
		ipv6Gateway:   profile.IPv6.Gateway,
		ipv6DNS:       strings.Join(profile.IPv6.DNS, ", "),
		autoconnect:   profile.Autoconnect,
	}
	if profile.MTU != 0 {
		editor.mtu = strconv.FormatUint(uint64(profile.MTU), 10)
	}
	editor.placeCursorAtEnd()
	return editor
}

func (e *ethernetEditor) handleKey(msg tea.KeyPressMsg) editorAction {
	key := msg.String()
	e.message = ""
	e.messageIsError = false

	switch key {
	case "ctrl+s":
		return editorActionSave
	case "esc":
		return editorActionCancel
	case "up", "k", "shift+tab":
		e.moveField(-1)
		return editorActionNone
	case "down", "j", "tab":
		e.moveField(1)
		return editorActionNone
	}

	switch e.field {
	case fieldIPv4Method:
		switch key {
		case "left", "h":
			e.ipv4Method = cycleIPMethod(e.ipv4Method, -1)
		case "right", "l", " ", "enter":
			e.ipv4Method = cycleIPMethod(e.ipv4Method, 1)
		}
		return editorActionNone

	case fieldIPv6Method:
		switch key {
		case "left", "h":
			e.ipv6Method = cycleIPMethod(e.ipv6Method, -1)
		case "right", "l", " ", "enter":
			e.ipv6Method = cycleIPMethod(e.ipv6Method, 1)
		}
		return editorActionNone

	case fieldAutoconnect:
		switch key {
		case "left", "right", "h", "l", " ", "enter":
			e.autoconnect = !e.autoconnect
		}
		return editorActionNone
	}

	if !e.isTextField(e.field) {
		return editorActionNone
	}

	switch key {
	case "enter":
		e.moveField(1)
	case "left":
		if e.textCursor > 0 {
			e.textCursor--
		}
	case "right":
		if e.textCursor < len([]rune(e.textValue(e.field))) {
			e.textCursor++
		}
	case "home", "ctrl+a":
		e.textCursor = 0
	case "end", "ctrl+e":
		e.placeCursorAtEnd()
	case "backspace":
		e.deleteBeforeCursor()
	case "delete":
		e.deleteAtCursor()
	case "ctrl+u":
		e.setTextValue(e.field, "")
		e.textCursor = 0
	default:
		if msg.Text != "" {
			e.insertText(msg.Text)
		}
	}

	return editorActionNone
}

func (e *ethernetEditor) buildProfile() (model.EthernetProfile, error) {
	profile := e.profile
	profile.Autoconnect = e.autoconnect

	ipv4 := model.IPProfileConfig{Method: e.ipv4Method}
	if e.ipv4Method == model.IPMethodManual {
		addresses, err := validation.ParseAddresses(e.ipv4Addresses, false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 addresses: %w", err)
		}
		gateway, err := validation.ParseGateway(e.ipv4Gateway, false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 gateway: %w", err)
		}
		ipv4.Addresses = addresses
		ipv4.Gateway = gateway
	}
	if e.ipv4Method != model.IPMethodDisabled {
		dns, err := validation.ParseDNS(e.ipv4DNS, false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 DNS: %w", err)
		}
		ipv4.DNS = dns
	}
	profile.IPv4 = ipv4

	ipv6 := model.IPProfileConfig{Method: e.ipv6Method}
	if e.ipv6Method == model.IPMethodManual {
		addresses, err := validation.ParseAddresses(e.ipv6Addresses, true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 addresses: %w", err)
		}
		gateway, err := validation.ParseGateway(e.ipv6Gateway, true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 gateway: %w", err)
		}
		ipv6.Addresses = addresses
		ipv6.Gateway = gateway
	}
	if e.ipv6Method != model.IPMethodDisabled {
		dns, err := validation.ParseDNS(e.ipv6DNS, true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 DNS: %w", err)
		}
		ipv6.DNS = dns
	}
	profile.IPv6 = ipv6

	mtu, err := validation.ParseMTU(e.mtu, e.ipv6Method != model.IPMethodDisabled)
	if err != nil {
		return model.EthernetProfile{}, err
	}
	profile.MTU = mtu

	if err := validation.ValidateEthernetProfile(profile); err != nil {
		return model.EthernetProfile{}, err
	}
	return profile, nil
}

func (e *ethernetEditor) render(width int, saving bool) string {
	if width < 42 {
		width = 42
	}

	var out strings.Builder
	out.WriteString(titleStyle.Render("Edit Ethernet · " + emptyFallback(e.profile.InterfaceName, "unknown")))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render(emptyFallback(e.profile.ID, "Saved Ethernet profile")))
	out.WriteString("\n\n")
	out.WriteString(warningStyle.Render("Saved settings are persistent. They are not applied to the live connection yet."))
	out.WriteString("\n\n")

	for field := editorField(0); field < editorFieldCount; field++ {
		if !e.fieldVisible(field) {
			continue
		}
		out.WriteString(e.renderField(field, width))
		out.WriteString("\n")
	}

	if e.message != "" {
		out.WriteString("\n")
		if e.messageIsError {
			out.WriteString(errorStyle.Render(e.message))
		} else {
			out.WriteString(goodStyle.Render(e.message))
		}
		out.WriteString("\n")
	}
	if saving {
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Saving profile..."))
		out.WriteString("\n")
	}

	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ move   ←/→ change option   type to edit   Ctrl-S save   Esc cancel"))
	return out.String()
}

func (e *ethernetEditor) renderField(field editorField, width int) string {
	label, value := e.fieldLabelValue(field)
	marker := "  "
	if field == e.field {
		marker = "› "
		if e.isTextField(field) {
			value = renderTextCursor(value, e.textCursor)
		}
	}

	line := fmt.Sprintf("%s%-18s %s", marker, label, value)
	if field == e.field {
		return selectedCardStyle.Width(cardContentWidth(width)).Render(line)
	}
	return cardStyle.Width(cardContentWidth(width)).Render(line)
}

func (e *ethernetEditor) fieldLabelValue(field editorField) (string, string) {
	switch field {
	case fieldIPv4Method:
		return "IPv4 method", methodLabel(e.ipv4Method)
	case fieldIPv4Addresses:
		return "IPv4 address/prefix", emptyFallback(e.ipv4Addresses, "example: 192.168.1.50/24")
	case fieldIPv4Gateway:
		return "IPv4 gateway", emptyFallback(e.ipv4Gateway, "optional")
	case fieldIPv4DNS:
		return "IPv4 DNS", emptyFallback(e.ipv4DNS, "automatic / none")
	case fieldIPv6Method:
		return "IPv6 method", methodLabel(e.ipv6Method)
	case fieldIPv6Addresses:
		return "IPv6 address/prefix", emptyFallback(e.ipv6Addresses, "example: 2001:db8::10/64")
	case fieldIPv6Gateway:
		return "IPv6 gateway", emptyFallback(e.ipv6Gateway, "optional")
	case fieldIPv6DNS:
		return "IPv6 DNS", emptyFallback(e.ipv6DNS, "automatic / none")
	case fieldAutoconnect:
		if e.autoconnect {
			return "Autoconnect", "On"
		}
		return "Autoconnect", "Off"
	case fieldMTU:
		return "MTU", emptyFallback(e.mtu, "Automatic")
	default:
		return "", ""
	}
}

func (e *ethernetEditor) moveField(direction int) {
	next := int(e.field)
	for attempts := 0; attempts < int(editorFieldCount); attempts++ {
		next += direction
		if next < 0 {
			next = int(editorFieldCount) - 1
		}
		if next >= int(editorFieldCount) {
			next = 0
		}
		field := editorField(next)
		if e.fieldVisible(field) {
			e.field = field
			e.placeCursorAtEnd()
			return
		}
	}
}

func (e *ethernetEditor) fieldVisible(field editorField) bool {
	switch field {
	case fieldIPv4Addresses, fieldIPv4Gateway:
		return e.ipv4Method == model.IPMethodManual
	case fieldIPv4DNS:
		return e.ipv4Method != model.IPMethodDisabled
	case fieldIPv6Addresses, fieldIPv6Gateway:
		return e.ipv6Method == model.IPMethodManual
	case fieldIPv6DNS:
		return e.ipv6Method != model.IPMethodDisabled
	default:
		return true
	}
}

func (e *ethernetEditor) isTextField(field editorField) bool {
	switch field {
	case fieldIPv4Addresses, fieldIPv4Gateway, fieldIPv4DNS,
		fieldIPv6Addresses, fieldIPv6Gateway, fieldIPv6DNS, fieldMTU:
		return true
	default:
		return false
	}
}

func (e *ethernetEditor) textValue(field editorField) string {
	switch field {
	case fieldIPv4Addresses:
		return e.ipv4Addresses
	case fieldIPv4Gateway:
		return e.ipv4Gateway
	case fieldIPv4DNS:
		return e.ipv4DNS
	case fieldIPv6Addresses:
		return e.ipv6Addresses
	case fieldIPv6Gateway:
		return e.ipv6Gateway
	case fieldIPv6DNS:
		return e.ipv6DNS
	case fieldMTU:
		return e.mtu
	default:
		return ""
	}
}

func (e *ethernetEditor) setTextValue(field editorField, value string) {
	switch field {
	case fieldIPv4Addresses:
		e.ipv4Addresses = value
	case fieldIPv4Gateway:
		e.ipv4Gateway = value
	case fieldIPv4DNS:
		e.ipv4DNS = value
	case fieldIPv6Addresses:
		e.ipv6Addresses = value
	case fieldIPv6Gateway:
		e.ipv6Gateway = value
	case fieldIPv6DNS:
		e.ipv6DNS = value
	case fieldMTU:
		e.mtu = value
	}
}

func (e *ethernetEditor) placeCursorAtEnd() {
	e.textCursor = len([]rune(e.textValue(e.field)))
}

func (e *ethernetEditor) insertText(text string) {
	value := []rune(e.textValue(e.field))
	cursor := e.textCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	insert := []rune(text)
	value = append(value[:cursor], append(insert, value[cursor:]...)...)
	e.setTextValue(e.field, string(value))
	e.textCursor = cursor + len(insert)
}

func (e *ethernetEditor) deleteBeforeCursor() {
	value := []rune(e.textValue(e.field))
	if e.textCursor <= 0 || len(value) == 0 {
		return
	}
	cursor := e.textCursor
	value = append(value[:cursor-1], value[cursor:]...)
	e.setTextValue(e.field, string(value))
	e.textCursor--
}

func (e *ethernetEditor) deleteAtCursor() {
	value := []rune(e.textValue(e.field))
	if e.textCursor < 0 || e.textCursor >= len(value) {
		return
	}
	cursor := e.textCursor
	value = append(value[:cursor], value[cursor+1:]...)
	e.setTextValue(e.field, string(value))
}

func cycleIPMethod(method model.IPMethod, direction int) model.IPMethod {
	methods := []model.IPMethod{
		model.IPMethodAuto,
		model.IPMethodManual,
		model.IPMethodDisabled,
	}
	index := 0
	for i, candidate := range methods {
		if method == candidate {
			index = i
			break
		}
	}
	index = (index + direction + len(methods)) % len(methods)
	return methods[index]
}

func methodLabel(method model.IPMethod) string {
	switch method {
	case model.IPMethodAuto:
		return "Automatic"
	case model.IPMethodManual:
		return "Manual"
	case model.IPMethodDisabled:
		return "Disabled"
	default:
		return string(method)
	}
}

func formatProfileAddresses(addresses []model.IPAddress) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, fmt.Sprintf("%s/%d", address.Address, address.Prefix))
	}
	return strings.Join(values, ", ")
}

func renderTextCursor(value string, cursor int) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	runes = append(runes[:cursor], append([]rune("▏"), runes[cursor:]...)...)
	return string(runes)
}
