/*
Package views provides Ghostbin's view model. Views are templates loaded
from a set of globbed files.

The view model provides five predefined template functions, and allows the
binding of additional template functions by way of the
GlobalFunctionProviderOption.

	global . <var>
		Fetches a variable from the global view data provider.
		NOTE: Only available if the Model was provided the GlobalDataProviderOption.
	local . <var>
		Fetches a variable from the local view data provider.
	now
		Returns the current time.
	subexec . <name>
		Renders a template and returns its HTML.
	subtemplate . <name>
		Renders a sibling view via subexec. When rendering a page
		"x", subtemplate . "y" will render template "x_y".

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

At execution time, bound views provide on the {{.}} template argument a
context object with the following methods

	.Obj
		Returns the object or objects (as a slice) that were provided
		to View.Exec().
	.Request
		Returns the *net/http.Request that triggered the rendering of
		this view.
	.With <key> <value>
		Returns a new context (for the same view, object, and request),
		with the provided key mapped to the provided value.

		.With is useful only in conjunction with .Value.
	.Value <key>
		Returns the value mapped to the provided key by way of .With.

USING .With AND .Value
----------------------

Since .With returns a new view context, it can be thought of as akin to
package context's .WithValue. .With attaches a key-value pair to a view
context without mutating the parent context's state. Therefore, only
descendent contexts can access that value.

Its primary use is to pass values across nested template invocations:

	{{template "enclose"}}
	{{- $t := .Value "subtemplate" -}}
	<div id="enclosed_{{$t}}">
	{{subexec . $t}}
	</div>
	{{end}}

	{{template "a" -}}
	Hello World
	{{- end}}

	{{template "b"}}
	{{with .With "subtemplate" "a" -}}
	{{template "enclose" .}}
	{{- end}}
	{{end}}

The template set above seen through `b' will render

	<div id="enclosed_a">
	Hello World
	</div>
*/
package views
