// Declare a global suite for the package.
package pyfmt_test

import (
	"testing"

	"github.com/dalibo/ldap2pg/internal/config"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/slog"
)

type Suite struct {
	suite.Suite
}

func Test(t *testing.T) {
	if testing.Verbose() {
		config.SetLoggingHandler(slog.LevelDebug, false)
	} else {
		config.SetLoggingHandler(slog.LevelWarn, false)
	}
	suite.Run(t, new(Suite))
}