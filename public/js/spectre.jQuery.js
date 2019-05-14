(function($){
	"use strict";
	$.fn.fillWithLineNumbers = function(lines, callback) {
		var lineNumberTrough = $(this[0]);
		if(lines === (0+lineNumberTrough.data("lines"))) return;

		var n="";
		var i = 0;
		for(i=0; i < lines; i++) {
			n += "<span>"+(i+1)+"</span>";
		}
		lineNumberTrough.html(n);

		lineNumberTrough.data("lines", lines);
		if(callback) callback();
	};
	$.fn.scrollMinimal = function() {
		/* From http://stackoverflow.com/questions/4217962/scroll-to-an-element-using-jquery */
		var cTop = this.offset().top;
		var cHeight = this.outerHeight(true);
		var windowTop = $(window).scrollTop();
		var visibleHeight = $(window).height();

		if (cTop < windowTop) {
			$(window).scrollTop(cTop);
		} else if (cTop + cHeight > windowTop + visibleHeight) {
			$(window).scrollTop(cTop - visibleHeight + cHeight);
		}
	};
	$.fn.onMediaQueryChanged = function(mediaQuery, callback) {
		var self = this;
		var mql = window.matchMedia(mediaQuery);
		var lastMqlMatch;
		var mqlListener = function(mql) {
			if(mql.matches === lastMqlMatch) return;
			callback.call(self, mql);
			lastMqlMatch = mql.matches;
			$(document).trigger("media-query-changed", mql);
		};
		mqlListener(mql);
		mql.addListener(mqlListener);
	};
	$.fn.serializeObject = function() {
		var self = this;
		var object = {};
		$.each($(self).serializeArray(), function(i, v) {
			object[v["name"]] = v["value"];
		});
		return object;
	};
})(jQuery);

/* From https://github.com/sprucemedia/jQuery.divPlaceholder.js */
(function ($) {
        $(document).on('change keydown keypress input', '*[data-placeholder]', function() {
                if (this.textContent) {
                        this.setAttribute('data-div-placeholder-content', 'true');
                }
                else {
                        this.removeAttribute('data-div-placeholder-content');
                }
        });
	$(function() {
		$("*[data-placeholder]").trigger("change");
	});
})(jQuery);
