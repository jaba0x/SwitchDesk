package aruba

import (
	"reflect"
	"testing"
)

const cxClientsDetail = `
Port Access Client Status Details:

Client 00:0c:29:9b:cc:d0, jdoe
============================
  Session Details
  ---------------
    Port          : 1/1/15
    Session Time  : 1203s
    IPv4 Address  : 10.3.0.21
    IPv6 Address  :
    Device Type   :

  VLAN Details
  ------------
    VLANs Assigned       : 10
    Access               : 10
    Native Untagged      :
    Allowed Trunk        :

  Authentication Details
  ----------------------
    Status          : dot1x Authenticated
    Auth Precedence : dot1x - Authenticated, mac-auth - Not attempted
    Auth History    : dot1x - Authenticated, 1190s ago
                      mac-auth - Unauthenticated, Session Timeout, 1201s ago

  Authorization Details
  ----------------------
    Role        : RoleLan
    Status      : Applied

Client 9c:8c:d8:01:02:03
============================
  Session Details
  ---------------
    Port          : 1/1/16
    Session Time  : 15s

  VLAN Details
  ------------
    VLANs Assigned       : 3,10,20-21
    Access               :
    Native Untagged      : 3
    Allowed Trunk        : 3,10,20-21

  Authentication Details
  ----------------------
    Status          : mac-auth Unauthenticated

  Authorization Details
  ----------------------
    Role        : RoleGuest
    Status      : Failed
`

const cxClientsTable = `
Port Access Clients

Status codes: d device-mode
-------------------------------------------------------------------------------
  Port      MAC-Address       Onboarding    Status        Role
                              Method
-------------------------------------------------------------------------------
  1/1/15    00:0c:29:9b:cc:d0 dot1x         Success       RoleLan
  1/1/16    9c8cd8-010203     mac-auth      In Progress   RoleGuest
`

func TestParsePortClients(t *testing.T) {
	got, ok := ParsePortClients(cxClientsDetail)
	want := []PortClient{
		{Port: "1/1/15", MAC: "00:0c:29:9b:cc:d0", User: "jdoe", IP: "10.3.0.21", Method: "dot1x", Status: "Authenticated", Role: "RoleLan", RoleStatus: "Applied", VLAN: "10", Session: "1203s"},
		{Port: "1/1/16", MAC: "9c:8c:d8:01:02:03", Method: "mac-auth", Status: "Unauthenticated", Role: "RoleGuest", RoleStatus: "Failed", VLAN: "3", Tagged: "3,10,20-21", Session: "15s"},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("detail:\n got %+v\nwant %+v", got, want)
	}

	got, ok = ParsePortClients(cxClientsTable)
	want = []PortClient{
		{Port: "1/1/15", MAC: "00:0c:29:9b:cc:d0", Method: "dot1x", Status: "Success", Role: "RoleLan"},
		{Port: "1/1/16", MAC: "9c:8c:d8:01:02:03", Method: "mac-auth", Status: "In Progress", Role: "RoleGuest"},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("table:\n got %+v\nwant %+v", got, want)
	}

	if got, ok := ParsePortClients("\nPort Access Clients\n\n  Port  MAC-Address  Onboarding  Status  Role\n"); !ok || len(got) != 0 {
		t.Errorf("empty table: %v %v", got, ok)
	}
	if _, ok := ParsePortClients("% Invalid input: port-access"); ok {
		t.Error("a rejected command must not count as read")
	}
}
