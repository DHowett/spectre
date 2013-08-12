/* From http://stackoverflow.com/questions/4217962/scroll-to-an-element-using-jquery */
(function($){
	$.fn.fillWithLineNumbers = function(lines, callback) {
		var lineNumberTrough = $(this[0])
		if(lines == lineNumberTrough.data("lines")) return;

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
})(jQuery);

$(function() {
	(function(){
		var controls = $("#paste-controls");
		var langbox = $("#langbox");
		if(controls.length == 0) return;

		if(langbox.length > 0) {
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
			newParent.prepend(controls);

			lastMqlMatch = mql.matches;

			$(document).trigger("media-query-changed");
		};
		mqlListener(mql);

		mql.addListener(mqlListener);
	})();
	(function(){
		var encModal = $("#encryptModal")
		if(encModal.length == 0) return;

		var encDataLabel = $("encryptionButton .button-data-label");

		encModal.modal({show: false});

		encModal.find("input[type='password']").keypress(function(e) {
			if(e.which == 13) {
				encModal.modal("hide");
				return false;
			}
		});

		encModal.on("shown", function() {
			$(this).find("input").eq(0).focus().select();
		});

		var setEncrypted = function(encrypted) {
			$("#encryptionIcon").removeClass("icon-lock icon-lock-open-alt").addClass(encrypted ? "icon-lock" : "icon-lock-open-alt");
			encDataLabel.text(encrypted ? "On" : "");
		};

		encModal.on("hidden", function() {
			setEncrypted($(this).find("input").val().length > 0);
		});

		$("#encryptionButton").on("click", function() {
			encModal.modal("show");
		});
	})();
	(function(){
		var expModal = $("#expireModal");
		if(expModal.length == 0) return;

		expModal.modal({show: false});

		var expInput = $("input[name='expire']");
		var expDataLabel = $("#expirationButton .button-data-label");

		var setExpirationSelected = function() {
			$(this).button('toggle');
			expInput.val($(this).data("value"));
			expDataLabel.text($(this).data("display-value"));
		};

		setExpirationSelected.call(expModal.find("button[data-value='"+expInput.val()+"']"));
		expModal.find("button[data-value]").on("click", setExpirationSelected)

		$("#expirationButton").on("click", function() {
			expModal.modal("show");
		});
	})();

	// Common for the following functions.
	var lineNumberTrough = $("#line-numbers");
	var code = $("#code"), codeeditor = $("#code-editor");

	(function(){
		if(lineNumberTrough.length == 0) return;

		if(code.length > 0) {
			var linebar = $(document.createElement('div'))
					.addClass("line-highlight-bar")
					.hide()
					.appendTo('body');
			var permabar = linebar
					.clone()
					.addClass("line-highlight-bar-permanent")
					.appendTo("body");

			var positionLinebar = function(linebar) {
				linebar
					.css("left", lineNumberTrough.outerWidth())
					.css("top", $(this).position().top + $(this).parent().position().top)
					.width(code.outerWidth())
					.show();
			}

			var setSelectedLineNumber = function(line) {
				if(typeof line != 'undefined') {
					permabar.data("cur-line", line);
					history.replaceState({"line":line}, "", "#L"+line);
				} else {
					permabar.removeData("cur-line");
					history.replaceState(null, "", "#");
				}
			}

			var lineFromHash = function(hash) {
				if(!hash) return undefined;
				var v = hash.match(/^#L(\d+)/);
				if(typeof v != 'undefined' && v.length > 0) {
					return v[1];
				}
				return undefined;
			}

			lineNumberTrough.fillWithLineNumbers((code.text().match(/\n/g)||[]).length+1, function() {
				lineNumberTrough.children().mouseenter(function() {
					positionLinebar.call(this, linebar);
				}).mouseleave(function() {
					linebar.hide();
				}).click(function() {
					var line = $(this).text();
					if(permabar.data("cur-line") == line) {
						setSelectedLineNumber(undefined);
						permabar.hide();
						return;
					}
					setSelectedLineNumber(line);
					positionLinebar.call(this, permabar);
				});

				$(window).on("load popstate", function(e) {
					var n = lineFromHash(window.location.hash);
					if(n) {
						var linespan = $("span:nth-child("+n+")", lineNumberTrough);
						if(linespan.length > 0) {
							setSelectedLineNumber(n);
							positionLinebar.call(linespan.get(0), permabar);
							linespan.scrollMinimal();
						}
					}
				});
			});
			$(window).on("resize", function(e) {
				$(linebar).width(code.outerWidth());
				$(permabar).width(code.outerWidth());
			});
			$(document).on("media-query-changed", function() {
				positionLinebar.call($("span:nth-child("+permabar.data("cur-line")+")", lineNumberTrough).get(0), permabar);
			});
		} else if(codeeditor.length > 0) {
			codeeditor.on("input propertychange", function() {
				lineNumberTrough.fillWithLineNumbers((codeeditor.val().match(/\n/g)||[]).length+1, function() {
					$(".textarea-height-wrapper").css("left", lineNumberTrough.outerWidth());
				});
			}).triggerHandler("input");
			$(document).on("media-query-changed", function() {
				codeeditor.triggerHandler("input");
			});
		}
	})();
	(function(){
		if(codeeditor.length > 0) {
			codeeditor.keydown(function(e) {
				if(e.keyCode == 9 && !e.ctrlKey && !e.altKey && !e.shiftKey) {
					var ends = [this.selectionStart, this.selectionEnd];
					this.value = this.value.substring(0, ends[0]) + "\t" + this.value.substring(ends[1], this.value.length);
					this.selectionStart = this.selectionEnd = ends[0] + 1;
					return false;
				}
			});
		}
	})();
	(function(){
		$('[autofocus]:not(:focus)').eq(0).focus();
	})();
});
