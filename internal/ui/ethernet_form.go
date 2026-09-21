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

type formField int

const (
	fieldIPv4Method formField = iota
	fieldIPv4Addresses
	fieldIPv4Gateway
	fieldIPv4DNS
	fieldIPv6Method
	fieldIPv6Addresses
	fieldIPv6Gateway
	fieldIPv6DNS
	fieldAutoconnect
	fieldMTU
	fieldSave
	fieldCancel
	formFieldCount
)

type formAction int

const (
	formActionNone formAction = iota
	formActionSave
	formActionCancel
)

type ethernetForm struct {
	profile     model.EthernetProfile
	field       formField
	ipv4Method  model.IPMethod
	ipv6Method  model.IPMethod
	autoconnect bool

	ipv4Addresses textinput.Model
	ipv4Gateway   textinput.Model
	ipv4DNS       textinput.Model
	ipv6Addresses textinput.Model
	ipv6Gateway   textinput.Model
	ipv6DNS       textinput.Model
	mtu           textinput.Model

	message        string
	messageIsError bool
}

func newEthernetForm(profile model.EthernetProfile) *ethernetForm {
	form := &ethernetForm{
		profile:     profile,
		ipv4Method:  profile.IPv4.Method,
		ipv6Method:  profile.IPv6.Method,
		autoconnect: profile.Autoconnect,
	}

	form.ipv4Addresses = newFormInput("192.168.1.50/24")
	form.ipv4Gateway = newFormInput("192.168.1.1")
	form.ipv4DNS = newFormInput("1.1.1.1, 9.9.9.9")
	form.ipv6Addresses = newFormInput("2001:db8::10/64")
	form.ipv6Gateway = newFormInput("2001:db8::1")
	form.ipv6DNS = newFormInput("2606:4700:4700::1111")
	form.mtu = newFormInput("Automatic")

	form.ipv4Addresses.SetValue(formatProfileAddresses(profile.IPv4.Addresses))
	form.ipv4Gateway.SetValue(profile.IPv4.Gateway)
	form.ipv4DNS.SetValue(strings.Join(profile.IPv4.DNS, ", "))
	form.ipv6Addresses.SetValue(formatProfileAddresses(profile.IPv6.Addresses))
	form.ipv6Gateway.SetValue(profile.IPv6.Gateway)
	form.ipv6DNS.SetValue(strings.Join(profile.IPv6.DNS, ", "))
	if profile.MTU != 0 {
		form.mtu.SetValue(strconv.FormatUint(uint64(profile.MTU), 10))
	}

	form.syncFocus()
	return form
}

func newFormInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 256
	input.SetWidth(44)
	return input
}

func (f *ethernetForm) handleKey(msg tea.KeyPressMsg) (formAction, tea.Cmd) {
	f.message = ""
	f.messageIsError = false

	switch msg.String() {
	case "esc":
		return formActionCancel, nil
	case "up", "shift+tab":
		return formActionNone, f.moveField(-1)
	case "down", "tab":
		return formActionNone, f.moveField(1)
	case "enter":
		switch f.field {
		case fieldSave:
			return formActionSave, nil
		case fieldCancel:
			return formActionCancel, nil
		default:
			return formActionNone, f.moveField(1)
		}
	}

	if f.isTextField(f.field) {
		input := f.inputForField(f.field)
		updated, cmd := input.Update(msg)
		*input = updated
		return formActionNone, cmd
	}

	switch f.field {
	case fieldIPv4Method:
		switch msg.String() {
		case "left":
			f.ipv4Method = cycleIPMethod(f.ipv4Method, -1)
		case "right", " ":
			f.ipv4Method = cycleIPMethod(f.ipv4Method, 1)
		}
	case fieldIPv6Method:
		switch msg.String() {
		case "left":
			f.ipv6Method = cycleIPMethod(f.ipv6Method, -1)
		case "right", " ":
			f.ipv6Method = cycleIPMethod(f.ipv6Method, 1)
		}
	case fieldAutoconnect:
		switch msg.String() {
		case "left", "right", " ":
			f.autoconnect = !f.autoconnect
		}
	}

	f.ensureVisibleField()
	f.syncFocus()
	return formActionNone, nil
}

func (f *ethernetForm) buildProfile() (model.EthernetProfile, error) {
	profile := f.profile
	profile.Autoconnect = f.autoconnect

	ipv4 := model.IPProfileConfig{Method: f.ipv4Method}
	if f.ipv4Method == model.IPMethodManual {
		addresses, err := validation.ParseAddresses(f.ipv4Addresses.Value(), false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 address: %w", err)
		}
		gateway, err := validation.ParseGateway(f.ipv4Gateway.Value(), false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 gateway: %w", err)
		}
		ipv4.Addresses = addresses
		ipv4.Gateway = gateway
	}
	if f.ipv4Method != model.IPMethodDisabled {
		dns, err := validation.ParseDNS(f.ipv4DNS.Value(), false)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv4 DNS: %w", err)
		}
		ipv4.DNS = dns
	}
	profile.IPv4 = ipv4

	ipv6 := model.IPProfileConfig{Method: f.ipv6Method}
	if f.ipv6Method == model.IPMethodManual {
		addresses, err := validation.ParseAddresses(f.ipv6Addresses.Value(), true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 address: %w", err)
		}
		gateway, err := validation.ParseGateway(f.ipv6Gateway.Value(), true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 gateway: %w", err)
		}
		ipv6.Addresses = addresses
		ipv6.Gateway = gateway
	}
	if f.ipv6Method != model.IPMethodDisabled {
		dns, err := validation.ParseDNS(f.ipv6DNS.Value(), true)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("IPv6 DNS: %w", err)
		}
		ipv6.DNS = dns
	}
	profile.IPv6 = ipv6

	mtu, err := validation.ParseMTU(f.mtu.Value(), f.ipv6Method != model.IPMethodDisabled)
	if err != nil {
		return model.EthernetProfile{}, err
	}
	profile.MTU = mtu

	if err := validation.ValidateEthernetProfile(profile); err != nil {
		return model.EthernetProfile{}, err
	}
	return profile, nil
}

func (f *ethernetForm) render(width int, saving bool) string {
	if width < 46 {
		width = 46
	}

	var out strings.Builder
	out.WriteString(titleStyle.Render("Ethernet settings · " + emptyFallback(f.profile.InterfaceName, "unknown")))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render(emptyFallback(f.profile.ID, "Saved Ethernet profile")))
	out.WriteString("\n\n")
	out.WriteString(warningStyle.Render("Save updates the persistent profile only; the live connection is not restarted."))
	out.WriteString("\n\n")

	for field := formField(0); field < formFieldCount; field++ {
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
	if saving {
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Saving profile..."))
		out.WriteString("\n")
	}

	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ or Tab move   ←/→ change options   Enter next/select   Esc cancel"))
	return out.String()
}

func (f *ethernetForm) renderField(field formField, width int) string {
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

func (f *ethernetForm) fieldLabelValue(field formField) (string, string) {
	switch field {
	case fieldIPv4Method:
		return "IPv4 method", optionValue(methodLabel(f.ipv4Method))
	case fieldIPv4Addresses:
		return "IPv4 address/prefix", f.ipv4Addresses.View()
	case fieldIPv4Gateway:
		return "IPv4 gateway", f.ipv4Gateway.View()
	case fieldIPv4DNS:
		return "IPv4 DNS", f.ipv4DNS.View()
	case fieldIPv6Method:
		return "IPv6 method", optionValue(methodLabel(f.ipv6Method))
	case fieldIPv6Addresses:
		return "IPv6 address/prefix", f.ipv6Addresses.View()
	case fieldIPv6Gateway:
		return "IPv6 gateway", f.ipv6Gateway.View()
	case fieldIPv6DNS:
		return "IPv6 DNS", f.ipv6DNS.View()
	case fieldAutoconnect:
		if f.autoconnect {
			return "Autoconnect", optionValue("On")
		}
		return "Autoconnect", optionValue("Off")
	case fieldMTU:
		return "MTU", f.mtu.View()
	case fieldSave:
		return "", goodStyle.Render("[ Save ]")
	case fieldCancel:
		return "", mutedStyle.Render("[ Cancel ]")
	default:
		return "", ""
	}
}

func (f *ethernetForm) moveField(direction int) tea.Cmd {
	next := int(f.field)
	for attempts := 0; attempts < int(formFieldCount); attempts++ {
		next += direction
		if next < 0 {
			next = int(formFieldCount) - 1
		}
		if next >= int(formFieldCount) {
			next = 0
		}
		field := formField(next)
		if f.fieldVisible(field) {
			f.field = field
			return f.syncFocus()
		}
	}
	return nil
}

func (f *ethernetForm) ensureVisibleField() {
	if f.fieldVisible(f.field) {
		return
	}
	_ = f.moveField(1)
}

func (f *ethernetForm) fieldVisible(field formField) bool {
	switch field {
	case fieldIPv4Addresses, fieldIPv4Gateway:
		return f.ipv4Method == model.IPMethodManual
	case fieldIPv4DNS:
		return f.ipv4Method != model.IPMethodDisabled
	case fieldIPv6Addresses, fieldIPv6Gateway:
		return f.ipv6Method == model.IPMethodManual
	case fieldIPv6DNS:
		return f.ipv6Method != model.IPMethodDisabled
	default:
		return true
	}
}

func (f *ethernetForm) isTextField(field formField) bool {
	switch field {
	case fieldIPv4Addresses, fieldIPv4Gateway, fieldIPv4DNS,
		fieldIPv6Addresses, fieldIPv6Gateway, fieldIPv6DNS, fieldMTU:
		return true
	default:
		return false
	}
}

func (f *ethernetForm) inputForField(field formField) *textinput.Model {
	switch field {
	case fieldIPv4Addresses:
		return &f.ipv4Addresses
	case fieldIPv4Gateway:
		return &f.ipv4Gateway
	case fieldIPv4DNS:
		return &f.ipv4DNS
	case fieldIPv6Addresses:
		return &f.ipv6Addresses
	case fieldIPv6Gateway:
		return &f.ipv6Gateway
	case fieldIPv6DNS:
		return &f.ipv6DNS
	case fieldMTU:
		return &f.mtu
	default:
		return nil
	}
}

func (f *ethernetForm) syncFocus() tea.Cmd {
	f.blurInputs()
	if !f.isTextField(f.field) {
		return nil
	}
	input := f.inputForField(f.field)
	if input == nil {
		return nil
	}
	return input.Focus()
}

func (f *ethernetForm) blurInputs() {
	f.ipv4Addresses.Blur()
	f.ipv4Gateway.Blur()
	f.ipv4DNS.Blur()
	f.ipv6Addresses.Blur()
	f.ipv6Gateway.Blur()
	f.ipv6DNS.Blur()
	f.mtu.Blur()
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

func optionValue(value string) string {
	return "‹ " + value + " ›"
}

func formatProfileAddresses(addresses []model.IPAddress) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, fmt.Sprintf("%s/%d", address.Address, address.Prefix))
	}
	return strings.Join(values, ", ")
}
