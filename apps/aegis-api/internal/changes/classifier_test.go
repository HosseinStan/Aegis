package changes

import (
	"slices"
	"strings"
	"testing"

	"company.local/aegis/apps/aegis-api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestClassifyTaxonomyAndPrecedence(t *testing.T) {
	iface := []string{"interface Gi1/0/1"}
	for _, test := range []struct {
		name, command             string
		context                   []string
		category, subcategory, id string
	}{
		{"hostname", "hostname LAB", nil, "system", "identity", "system.hostname"},
		{"boot", "boot system flash:ios.bin", nil, "system", "boot", "system.boot"},
		{"global service", "service timestamps log datetime", nil, "system", "global_service", "system.global_service"},
		{"crypto key is not VPN", "crypto key generate rsa modulus 2048", nil, "system", "crypto_key", "system.crypto_key"},
		{"interface definition", "interface Gi1/0/1", nil, "interface", "definition", "interface.definition"},
		{"interface range", "interface range Gi1/0/1-4", nil, "interface", "definition", "interface.definition"},
		{"description", "description Finance", iface, "interface", "description", "interface.description"},
		{"shutdown", "shutdown", iface, "interface", "admin_state", "interface.admin_state"},
		{"IPv4 address", "ip address 192.0.2.1 255.255.255.0", iface, "interface", "addressing", "interface.addressing"},
		{"IPv6 address", "ipv6 address 2001:db8::1/64", iface, "interface", "addressing", "interface.addressing"},
		{"unnumbered", "ip unnumbered Loopback0", iface, "interface", "addressing", "interface.addressing"},
		{"generic interface", "unknown-child", iface, "interface", "generic", "interface.generic"},
		{"VLAN definition", "vlan 20", nil, "switching", "vlan", "switching.vlan"},
		{"VLAN descendant", "name USERS", []string{"vlan 20", "unknown nested-mode"}, "switching", "vlan", "switching.vlan.context"},
		{"switchport beats interface", "switchport access vlan 20", iface, "switching", "switchport", "switching.switchport"},
		{"spanning tree", "spanning-tree portfast", iface, "switching", "spanning_tree", "switching.spanning_tree"},
		{"channel group", "channel-group 1 mode active", iface, "switching", "etherchannel", "switching.etherchannel"},
		{"port channel", "port-channel load-balance src-dst-ip", nil, "switching", "etherchannel", "switching.etherchannel"},
		{"VTP", "vtp mode transparent", nil, "switching", "vtp", "switching.vtp"},
		{"static IPv4 route", "ip route 192.0.2.0 255.255.255.0 198.51.100.1", nil, "routing", "static_route", "routing.static_route"},
		{"static IPv6 route", "ipv6 route 2001:db8::/64 2001:db8:1::1", nil, "routing", "static_route", "routing.static_route"},
		{"BGP definition", "router bgp 65000", nil, "routing", "bgp", "routing.bgp"},
		{"BGP full ancestry", "neighbor 192.0.2.1 activate", []string{"router bgp 65000", "address-family ipv4"}, "routing", "bgp", "routing.bgp.context"},
		{"OSPF definition", "router ospf 1", nil, "routing", "ospf", "routing.ospf"},
		{"OSPFv3 definition", "router ospfv3 1", nil, "routing", "ospf", "routing.ospf"},
		{"IPv6 OSPF definition", "ipv6 router ospf 1", nil, "routing", "ospf", "routing.ospf"},
		{"OSPF interface beats addressing", "ip ospf 1 area 0", iface, "routing", "ospf", "routing.ospf"},
		{"IPv6 OSPF interface", "ipv6 ospf 1 area 0", iface, "routing", "ospf", "routing.ospf"},
		{"OSPF ancestor", "network 192.0.2.0 0.0.0.255 area 0", []string{"router ospf 1"}, "routing", "ospf", "routing.ospf.context"},
		{"OSPFv3 ancestor", "router-id 192.0.2.1", []string{"router ospfv3 1", "address-family ipv6"}, "routing", "ospf", "routing.ospf.context"},
		{"EIGRP definition", "router eigrp 100", nil, "routing", "eigrp", "routing.eigrp"},
		{"EIGRP ancestor", "network 192.0.2.0", []string{"router eigrp 100", "address-family ipv4"}, "routing", "eigrp", "routing.eigrp.context"},
		{"RIP definition", "router rip", nil, "routing", "rip", "routing.rip"},
		{"RIP ancestor", "version 2", []string{"router rip"}, "routing", "rip", "routing.rip.context"},
		{"ISIS definition", "router isis CORE", nil, "routing", "isis", "routing.isis"},
		{"ISIS ancestor", "net 49.0001.0000.0000.0001.00", []string{"router isis CORE"}, "routing", "isis", "routing.isis.context"},
		{"route map", "route-map EXPORT permit 10", nil, "routing", "route_policy", "routing.route_policy"},
		{"route map descendant", "set local-preference 200", []string{"route-map EXPORT permit 10"}, "routing", "route_policy", "routing.route_policy.context"},
		{"prefix list IPv4", "ip prefix-list PREFIX permit 192.0.2.0/24", nil, "routing", "route_filter", "routing.route_filter"},
		{"prefix list IPv6", "ipv6 prefix-list PREFIX permit 2001:db8::/64", nil, "routing", "route_filter", "routing.route_filter"},
		{"unknown router is not BGP substring", "router bgp-test", nil, "routing", "generic", "routing.generic"},
		{"generic router inheritance", "new-protocol-option", []string{"router future-protocol", "address-family opaque"}, "routing", "generic", "routing.generic.context"},
		{"specific ancestor before generic router", "neighbor 192.0.2.1 activate", []string{"router bgp 65000", "router unknown"}, "routing", "bgp", "routing.bgp.context"},
		{"numbered ACL", "access-list 100 permit ip any any", nil, "access_control", "acl", "access_control.acl"},
		{"named IPv4 ACL", "ip access-list extended USERS", nil, "access_control", "acl", "access_control.acl"},
		{"named IPv6 ACL", "ipv6 access-list USERS", nil, "access_control", "acl", "access_control.acl"},
		{"ACL full ancestry", "permit tcp any any eq 443", []string{"ip access-list extended USERS", "remark USERS"}, "access_control", "acl", "access_control.acl.context"},
		{"ACL binding beats interface", "ip access-group USERS in", iface, "access_control", "acl_binding", "access_control.acl_binding"},
		{"IPv6 ACL binding", "ipv6 traffic-filter USERS in", iface, "access_control", "acl_binding", "access_control.acl_binding"},
		{"object group", "object-group network USERS", nil, "access_control", "object_group", "access_control.object_group"},
		{"object group descendant", "network-object 192.0.2.0 255.255.255.0", []string{"object-group network USERS"}, "access_control", "object_group", "access_control.object_group.context"},
		{"control plane", "control-plane", nil, "access_control", "control_plane", "access_control.control_plane"},
		{"control plane binding beats QoS", "service-policy input CONTROL", []string{"control-plane", "unknown nested-mode"}, "access_control", "control_plane", "access_control.control_plane.context"},
		{"control plane descendant", "unknown child", []string{"control-plane"}, "access_control", "control_plane", "access_control.control_plane.context"},
		{"AAA", "aaa new-model", nil, "aaa", "global", "aaa.global"},
		{"enable secret", "enable secret <redacted>", nil, "aaa", "global", "aaa.global"},
		{"enable password", "enable password <redacted>", nil, "aaa", "global", "aaa.global"},
		{"local user redacted", "username admin privilege 15 secret <redacted>", nil, "aaa", "local_user", "aaa.local_user"},
		{"RADIUS definition", "radius server LAB", nil, "aaa", "radius", "aaa.radius"},
		{"legacy RADIUS", "radius-server host 192.0.2.1 key <redacted>", nil, "aaa", "radius", "aaa.radius"},
		{"RADIUS redacted nested key", "key <redacted>", []string{"radius server LAB", "address ipv4 192.0.2.1"}, "aaa", "radius", "aaa.radius.context"},
		{"TACACS definition", "tacacs server LAB", nil, "aaa", "tacacs", "aaa.tacacs"},
		{"legacy TACACS", "tacacs-server host 192.0.2.1 key <redacted>", nil, "aaa", "tacacs", "aaa.tacacs"},
		{"TACACS nested key", "key <redacted>", []string{"tacacs server LAB"}, "aaa", "tacacs", "aaa.tacacs.context"},
		{"line login", "login authentication VTY", []string{"line vty 0 4"}, "aaa", "line_auth", "aaa.line_auth"},
		{"line password", "password <redacted>", []string{"line console 0"}, "aaa", "line_auth", "aaa.line_auth"},
		{"line authorization", "authorization exec VTY", []string{"line vty 0 4"}, "aaa", "line_auth", "aaa.line_auth"},
		{"line accounting", "accounting commands 15 VTY", []string{"line vty 0 4"}, "aaa", "line_auth", "aaa.line_auth"},
		{"VTY definition", "line vty 0 4", nil, "management", "remote_access", "management.remote_access"},
		{"transport input", "transport input ssh", []string{"line vty 0 4"}, "management", "remote_access", "management.remote_access"},
		{"transport output", "transport output ssh", []string{"line vty 0 4"}, "management", "remote_access", "management.remote_access"},
		{"VTY inheritance", "exec-timeout 10", []string{"line vty 0 4"}, "management", "remote_access", "management.remote_access.context"},
		{"SSH IPv4", "ip ssh version 2", nil, "management", "remote_access", "management.remote_access"},
		{"SSH", "ssh timeout 60", nil, "management", "remote_access", "management.remote_access"},
		{"console definition", "line console 0", nil, "management", "console", "management.console"},
		{"console inheritance", "exec-timeout 10", []string{"line console 0"}, "management", "console", "management.console.context"},
		{"HTTP server", "ip http server", nil, "management", "web", "management.web"},
		{"HTTPS server", "ip http secure-server", nil, "management", "web", "management.web"},
		{"domain name", "ip domain-name lab.example", nil, "management", "dns", "management.dns"},
		{"name server", "ip name-server 192.0.2.1", nil, "management", "dns", "management.dns"},
		{"NTP", "ntp server 192.0.2.1", nil, "management", "ntp", "management.ntp"},
		{"SNMP sanitized", "snmp-server community <redacted>", nil, "monitoring", "snmp", "monitoring.snmp"},
		{"SNMPv3 sanitized", "snmp-server user admin GROUP auth <redacted>", nil, "monitoring", "snmp", "monitoring.snmp"},
		{"logging", "logging host 192.0.2.10", nil, "monitoring", "logging", "monitoring.logging"},
		{"telemetry", "telemetry ietf subscription 1", nil, "monitoring", "telemetry", "monitoring.telemetry"},
		{"flow exporter", "flow exporter EXPORT", nil, "monitoring", "flow", "monitoring.flow"},
		{"flow monitor", "flow monitor MONITOR", nil, "monitoring", "flow", "monitoring.flow"},
		{"IP flow", "ip flow ingress", iface, "monitoring", "flow", "monitoring.flow"},
		{"netflow", "netflow export destination 192.0.2.1", nil, "monitoring", "flow", "monitoring.flow"},
		{"DHCP", "ip dhcp pool USERS", nil, "services", "dhcp", "services.dhcp"},
		{"DHCP beats global service", "service dhcp", nil, "services", "dhcp", "services.dhcp"},
		{"helper beats interface", "ip helper-address 192.0.2.20", iface, "services", "dhcp", "services.dhcp"},
		{"NAT beats interface", "ip nat inside", iface, "services", "nat", "services.nat"},
		{"IP SLA", "ip sla 1", nil, "services", "ip_sla", "services.ip_sla"},
		{"tracking", "track 1 ip sla 1 reachability", nil, "services", "tracking", "services.tracking"},
		{"ISAKMP", "crypto isakmp policy 10", nil, "crypto_vpn", "ike_isakmp", "crypto_vpn.ike_isakmp"},
		{"ISAKMP sanitized key", "crypto isakmp key <redacted> address 192.0.2.1", nil, "crypto_vpn", "ike_isakmp", "crypto_vpn.ike_isakmp"},
		{"IKEv2", "crypto ikev2 profile LAB", nil, "crypto_vpn", "ike_isakmp", "crypto_vpn.ike_isakmp"},
		{"IPsec", "crypto ipsec transform-set LAB esp-aes", nil, "crypto_vpn", "ipsec", "crypto_vpn.ipsec"},
		{"crypto map", "crypto map LAB 10 ipsec-isakmp", nil, "crypto_vpn", "crypto_map", "crypto_vpn.crypto_map"},
		{"crypto map inheritance", "set peer 192.0.2.1", []string{"crypto map LAB 10 ipsec-isakmp"}, "crypto_vpn", "crypto_map", "crypto_vpn.crypto_map.context"},
		{"VPN tunnel", "tunnel protection ipsec profile LAB", iface, "crypto_vpn", "tunnel", "crypto_vpn.tunnel"},
		{"dynamic VPN map", "crypto dynamic-map LAB 10", nil, "crypto_vpn", "generic", "crypto_vpn.generic"},
		{"class map", "class-map match-any VOICE", nil, "qos", "class_map", "qos.class_map"},
		{"class map descendant", "match dscp ef", []string{"class-map match-any VOICE"}, "qos", "class_map", "qos.class_map.context"},
		{"policy map", "policy-map WAN", nil, "qos", "policy_map", "qos.policy_map"},
		{"QoS full ancestry", "priority percent 20", []string{"policy-map WAN", "class VOICE"}, "qos", "policy_map", "qos.policy_map.context"},
		{"service policy beats interface", "service-policy input WAN", iface, "qos", "service_policy", "qos.service_policy"},
		{"generic QoS", "qos pre-classify", iface, "qos", "generic", "qos.generic"},
		{"MLS QoS", "mls qos trust dscp", iface, "qos", "generic", "qos.generic"},
		{"HSRP beats interface", "standby 10 ip 192.0.2.1", iface, "redundancy", "hsrp", "redundancy.hsrp"},
		{"VRRP", "vrrp 10 ip 192.0.2.1", iface, "redundancy", "vrrp", "redundancy.vrrp"},
		{"GLBP", "glbp 10 ip 192.0.2.1", iface, "redundancy", "glbp", "redundancy.glbp"},
		{"unknown", "unknown totally-new syntax", nil, "other", "unclassified", "fallback.unclassified"},
		{"not any crypto is VPN", "crypto unknown syntax", nil, "other", "unclassified", "fallback.unclassified"},
		{"token not substring", "xip route 192.0.2.0", nil, "other", "unclassified", "fallback.unclassified"},
		{"ancestor token not substring", "neighbor 192.0.2.1 activate", []string{"router bgp-test"}, "routing", "generic", "routing.generic.context"},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := Classification{test.category, test.subcategory, test.id}
			before := slices.Clone(test.context)
			require.Equal(t, want, Classify(test.context, test.command))
			require.Equal(t, before, test.context)
			// Positive/negative, case, whitespace, and parent normalization preserve
			// classification only; the persisted input is never rewritten.
			require.Equal(t, want, Classify(test.context, "no "+test.command))
			normalizedParents := make([]string, len(test.context))
			for i, parent := range test.context {
				normalizedParents[i] = " NO\t" + strings.ToUpper(strings.Join(strings.Fields(parent), "  \t"))
			}
			require.Equal(t, want, Classify(normalizedParents, " \t"+strings.ToUpper(strings.Join(strings.Fields(test.command), "  \t"))+"\t "))
			require.NotEmpty(t, want.RuleID)
			require.NotEqual(t, domain.ChangeCategoryUnclassified, want.Category)
		})
	}
}

func TestClassifyIgnoresOperationAndConfigurationValues(t *testing.T) {
	for _, command := range []string{"username admin secret <redacted>", "username changed-user secret <redacted>"} {
		add := domain.ChangeEventItem{Operation: domain.ChangeOperationAdd, Command: command, Sensitive: true}
		remove := domain.ChangeEventItem{Operation: domain.ChangeOperationRemove, Command: command, Sensitive: false}
		require.Equal(t, Classify(add.ContextPath, add.Command), Classify(remove.ContextPath, remove.Command))
		require.Equal(t, Classification{"aaa", "local_user", "aaa.local_user"}, Classify(nil, command))
	}
	for _, path := range [][]string{{"router bgp 65000", "address-family ipv4"}, {"router bgp 65123", "address-family ipv6"}} {
		require.Equal(t, Classification{"routing", "bgp", "routing.bgp.context"}, Classify(path, "neighbor 192.0.2.123 activate"))
	}
}

func TestClassifyTotalAndOneLeadingNo(t *testing.T) {
	for _, command := range []string{"", " \t\r\n", "no", "no no ip route 192.0.2.0", "\x00\xff malformed", "<redacted>", "IP ROUTER-TEST"} {
		require.Equal(t, Classification{"other", "unclassified", "fallback.unclassified"}, Classify(nil, command))
	}
	for _, rules := range [][]classificationRule{commandClassificationRules, contextClassificationRules, globalClassificationRules} {
		for _, rule := range rules {
			require.NotEmpty(t, rule.result.RuleID)
			for _, r := range rule.result.RuleID {
				require.True(t, (r >= 'a' && r <= 'z') || r == '.' || r == '_', "rule identifiers must be static technical labels")
			}
		}
	}
}

func FuzzClassifyTotalAndDeterministic(f *testing.F) {
	f.Add("router bgp 65000\naddress-family ipv4", "neighbor 192.0.2.1 activate")
	f.Add("radius server LAB", "key <redacted>")
	f.Add("\xff\x00", "\xfe no")
	f.Fuzz(func(t *testing.T, context, command string) {
		path := strings.Split(context, "\n")
		result := Classify(path, command)
		if result != Classify(path, command) || result.Category == "" || result.Subcategory == "" ||
			result.RuleID == "" || result.Category == domain.ChangeCategoryUnclassified {
			t.Fatal("classifier failed totality or determinism")
		}
	})
}
