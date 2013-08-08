/* From http://stackoverflow.com/questions/4217962/scroll-to-an-element-using-jquery */
jQuery.fn.scrollMinimal = function() {
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

$(function() {
	(function(){
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
			newParent.prepend(controls);

			lastMqlMatch = mql.matches;

			$(document).trigger("media-query-changed");
		};
		mqlListener(mql);

		mql.addListener(mqlListener);
	})();
	(function(){
		var encModal = $("#encryptModal")
		if(!encModal) return;

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

		encModal.on("hidden", function() {
			encrypted = $(this).find("input").val().length > 0;
			$("#encryptionIcon").removeClass("icon-lock icon-lock-open-alt").addClass(encrypted ? "icon-lock" : "icon-lock-open-alt");
		});

		$("#encryptionButton").on("click", function() {
			encModal.modal("show");
		});
	})();
	(function(){
		var expModal = $("#expireModal")
		if(!expModal) return;

		var expInput = $("input[name='expire']")

		expModal.modal({show: false});

		expModal.find("button[data-value='"+expInput.val()+"']").button('toggle');
		expModal.find("button[data-value]").on("click", function() {
			expInput.val($(this).data("value"));
		})

		$("#expirationButton").on("click", function() {
			expModal.modal("show");
		});
	})();
	(function(){
		var ln=$("#line-numbers");
		if(!ln) return;

		var curLines = -1;

		var fillForLines = function(lines, callback) {
			if(lines == curLines) return;
			var n="";
			var i = 0;
			for(i=0; i < lines; i++) {
				n += "<span>"+(i+1)+"</span>";
			}
			ln.html(n);

			curLines = lines;
			if(callback) callback();
		}

		if($("#code").length > 0) {
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
					.css("left", ln.outerWidth())
					.css("top", $(this).position().top + $(this).parent().position().top)
					.width($("#code").outerWidth())
					.show();
			}
			fillForLines(($("#code").text().match(/\n/g)||[]).length+1, function() {
				ln.children().mouseenter(function() {
					positionLinebar.call(this, linebar);
				}).mouseleave(function() {
					linebar.hide();
				}).click(function() {
					var line = $(this).text();
					if(history.state && history.state.line == line) {
						permabar.hide();
						history.replaceState({}, "", "#");
						return
					}
					positionLinebar.call(this, permabar);
					history.replaceState({"line":line}, "", "#L" + line);
				});

				if(window.location.hash) {
					var v = window.location.hash.match(/^#L(\d+)/);
					if(v) {
						var n = v[1];
						var linespan = $("span:nth-child("+n+")", ln);
						positionLinebar.call(linespan.get(0), permabar);
						history.replaceState({"line":n}, "", "#L" + n);
						linespan.scrollMinimal();
					}
				}
			});
			$(window).on("resize", function(e) {
				$(permabar).width($("#code").outerWidth());
			});
			$(document).on("media-query-changed", function() {
				positionLinebar.call($("span:nth-child("+history.state.line+")", ln).get(0), permabar);
			});
		} else if($("#code-editor").length > 0) {
			$("#code-editor").on("input propertychange", function() {
				fillForLines(($("#code-editor").val().match(/\n/g)||[]).length+1, function() {
					$(".textarea-height-wrapper").css("left", ln.outerWidth());
				});
			}).triggerHandler("input");
			$(document).on("media-query-changed", function() {
				$("#code-editor").triggerHandler("input");
			});
		}
	})();
	(function(){
		$('[autofocus]:not(:focus)').eq(0).focus();
	})();
	(function(){
		var ce = $("#code-editor");
		if(ce.length > 0) {
			ce.keydown(function(e) {
				if(e.keyCode == 9) {
					var ends = [this.selectionStart, this.selectionEnd];
					this.value = this.value.substring(0, ends[0]) + "\t" + this.value.substring(ends[1], this.value.length);
					this.selectionStart = this.selectionEnd = ends[0] + 1;
					return false;
				}
			});
		}
	})();
});
