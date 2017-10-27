package forward

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPv6Fix(t *testing.T) {
	assert.Equal(t, "", ipv6fix(""))
	assert.Equal(t, "127.0.0.1", ipv6fix("127.0.0.1"))
	assert.Equal(t, "10.13.14.15", ipv6fix("10.13.14.15"))
	assert.Equal(t, "fe80::d806:a55d:eb1b:49cc", ipv6fix(`fe80::d806:a55d:eb1b:49cc%vEthernet (vmxnet3 Ethernet Adapter - Virtual Switch)`))
	assert.Equal(t, "fe80::1", ipv6fix(`fe80::1`))
	assert.Equal(t, "2000::", ipv6fix(`2000::`))
	assert.Equal(t, "2001:3452:4952:2837::", ipv6fix(`2001:3452:4952:2837::`))
}
