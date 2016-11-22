/*
Package views provides Ghostbin's view model. Views are templates loaded
from a set of globbed files.

The view model provides four predefined template functions, and disallows
the creation of additional bound functions.

	global . <var>
		Fetches a variable from the global view data provider.
	local . <var>
		Fetches a variable from the local view data provider.
	now
		Returns the current time.
	subtemplate . <name>
		Renders a sibling view and returns its HTML.

A view model alone is not useful; it can contain any number of
templates but provides no way to use them. This is where binding comes
in.

Once a single view is bound via model.Bind(name, dataProvider), it can
be used in perpetuity as a handle to the open template with its bound
name. It will look up `local` variables from its bound data provider,
and `global` variables from its parent's bound data provider.

Views can be bound with string names, in which case they are simply
executed by name (and subtemplate will not function properly), or with
the special type views.PageID. A view bound by way of views.PageID will
render the special `tmpl_page` template, passing the bound name along in
{{.page}}. {{.page}} is crucial to the operation of subtemplate; for a
given page "A", {{subtemplate "X"}} will render the template named
"A_X".
*/
package views
