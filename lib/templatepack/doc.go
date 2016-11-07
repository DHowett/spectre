/*
Package templatepack provides for the management of a set of linked templates.

The templates loaded by this package must altogether represent a full set of pages, partials, and
rendered collateral. The page entry point is `tmpl_page'. `tmpl_page` will be furnished with {{.Page}},
{{.Obj}} and {{.Request}}, which it can use to dispatch the renderer to the appropriate page template.

Predefined global functions are set out in text/template and html/template, with additional functions
as follows.

	subtemplate . X
		Returns the result of executing the template named `{{.Page}}_x'
	partial . X
		Returns formatted HTML
			<div class="partial_container_X">
				{{template partial_X .}}
			</div>
	now
		Returns the current time as a time.Time
*/
package templatepack
