$(function() {
	var controls = $("#paste-controls");
	var langbox = $("#langbox");
	if(!controls) return;

	if(langbox) {
		langbox.select2();
		langbox.select2("val", langbox.data("selected"));
	}

	var mql = window.matchMedia("screen and (max-width: 767px)");
	var lastMqlMatch = undefined;
	var mqlListener = function(mql) {
		if(mql.matches === lastMqlMatch) return;
		controls.detach();
		var newParent = undefined;
		if(mql.matches) {
			newParent = $("#phone-paste-control-container");
		} else {
			newParent = $("#desktop-paste-control-container");
		}
		newParent.empty();
		newParent.append(controls);

		lastMqlMatch = mql.matches;
	};
	mqlListener(mql);

	mql.addListener(mqlListener);
});
