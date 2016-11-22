package views

import (
	"html/template"

	"github.com/Sirupsen/logrus"
)

// ModelOption represents a functional option for configuring a View
// Model.
type ModelOption func(*Model) error

// GlobalDataProviderOption binds the `global` template function to the
// supplied data provider.
func GlobalDataProviderOption(provider DataProvider) ModelOption {
	return func(m *Model) error {
		m.baseTemplate.Funcs(template.FuncMap{
			"global": varFromDataProvider(provider),
		})
		return nil
	}
}

// GlobalFunctionsOption binds the template functions yielded by the
// supplied function provider.
func GlobalFunctionsOption(provider FunctionProvider) ModelOption {
	return func(m *Model) error {
		m.baseTemplate.Funcs(template.FuncMap(provider.GetViewFunctions()))
		return nil
	}
}

// FieldLoggingOption enables logging to a logrus-enabled stream.
func FieldLoggingOption(logger logrus.FieldLogger) ModelOption {
	return func(m *Model) error {
		m.logger = logger
		return nil
	}
}
