//go:build unix

package main

import (
	"fmt"

	"vpntoris-tray/internal/fortihelper"
)

func nativeIPSecSupported(config VPNConfig) bool {
	return config.Type == "ipsec" && config.IPSec != nil && nativeHelperReady()
}

func nativeIPSecNeedsOTP(name string) bool {
	status, err := nativeFortiStatus(name)
	return err == nil && status.State == "waiting-otp"
}

func nativeIPSecConnect(config VPNConfig) error {
	settings := config.IPSec
	ikePRF := settings.IKEPRF
	if settings.IKEVersion == 1 {
		ikePRF = ""
	}
	ikeProposals, err := buildProposals(settings.IKEEncryption, settings.IKEIntegrity, ikePRF, settings.DHGroups, true)
	if err != nil {
		return err
	}
	pfs := ""
	if settings.PFS {
		pfs = settings.PFSGroups
	}
	espItems := make([]string, 0)
	if len(settings.Phase2Proposals) > 0 {
		for _, proposal := range settings.Phase2Proposals {
			value, proposalErr := buildProposals(proposal.Encryption, proposal.Authentication, "", pfs, false)
			if proposalErr != nil {
				return proposalErr
			}
			espItems = append(espItems, value)
		}
	} else {
		value, proposalErr := buildProposals(settings.ESPEncryption, settings.ESPIntegrity, "", pfs, false)
		if proposalErr != nil {
			return proposalErr
		}
		espItems = append(espItems, value)
	}
	routes, err := parseRoutes(config.Routes)
	if err != nil {
		return err
	}
	values := make([]string, 0, len(routes))
	for _, route := range routes {
		values = append(values, fmt.Sprintf("%s/%d", route.network, route.prefix))
	}
	authMode := settings.AuthMode
	if settings.IKEVersion == 1 && authMode == "eap" {
		authMode = "xauth"
	}
	fragmentation := settings.Fragmentation
	if fragmentation == "" {
		fragmentation = "yes"
	}
	dpdAction := settings.DPDAction
	if dpdAction == "" {
		dpdAction = "restart"
	}
	request := fortihelper.Request{Action: fortihelper.ActionStart, Profile: nativeProfileID(config.Name), Protocol: fortihelper.ProtocolIPSec, Host: config.Host, Username: config.User, Password: config.Password, TwoFactor: config.TwoFactor, Routes: values, IPSec: &fortihelper.IPSecRequest{Version: settings.IKEVersion, AuthMode: authMode, PreSharedKey: settings.PreSharedKey, LocalID: settings.LocalID, RemoteID: settings.RemoteID, ModeConfig: settings.ModeConfig, Aggressive: settings.IKEMode == "aggressive", MOBIKE: settings.MOBIKE, ForceEncap: settings.ForceEncap, Fragmentation: fragmentation, DPDAction: dpdAction, DPDDelay: max(settings.DPDDelay, 30), DPDTimeout: max(settings.DPDTimeout, 150), IKELifetime: max(settings.IKELifetime, 28800), ChildLifetime: max(settings.ChildLifetime, 3600), ReplayWindow: max(settings.ReplayWindow, 32), IKEProposals: ikeProposals, ESPProposals: joinNonEmpty(espItems)}}
	response, err := nativeFortiRequest(request)
	request.Password = ""
	request.IPSec.PreSharedKey = ""
	if err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

func joinNonEmpty(values []string) string {
	result := ""
	for _, value := range values {
		if value == "" {
			continue
		}
		if result != "" {
			result += ","
		}
		result += value
	}
	return result
}
