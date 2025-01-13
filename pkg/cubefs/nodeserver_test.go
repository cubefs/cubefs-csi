package cubefs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNodeServer(t *testing.T) {
	driver, _ := NewMockupDriver()
	svc := NewNodeServer(driver)
	assert.NotNil(t, svc)
}
