/* From http://stackoverflow.com/questions/4217962/scroll-to-an-element-using-jquery */
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
})(jQuery);

(function(window) {
	"use strict";
	window.Ghostbin = function() {
		var _s2Languages;
		var _languageMap;
		return {
			formatDuration: function(seconds) {
				seconds = seconds | 0;
				if(seconds < 60) {
					return ""+seconds+"s";
				} else if(seconds < 3600) {
					return ""+((seconds/60)|0)+"m"+((seconds%60)|0)+"s";
				} else { 
					// if(seconds < 86400) {
					return ""+((seconds/3600)|0)+"h"+(((seconds%3600)/60)|0)+"m"+((seconds%60)|0)+"s";
				}
			},
			loadLanguages: function() {
				var s2Languages = {
					more: false,
					results: [],
				};
				var langmap = {};
				$.ajax({
					url: "/languages.json",
					async: false,
					dataType: "json",
					cache: true,
					success: function(_languages) {
						$.each(_languages, function(i,cat) {
							var s2cat = cat;
							s2cat.text = cat.name;
							s2cat.children = cat.languages;
							$.each(cat.languages, function(i, lang) {
								lang.text = lang.name;

								langmap[lang.id] = lang;
								if(lang.alt_ids) {
									$.each(lang.alt_ids, function(i, n) { langmap[n] = lang; });
								}
							});
							s2Languages.results.push(s2cat);
						});
					}
				});
				_languageMap = langmap;
				_s2Languages = s2Languages;
				return;
			},
			languagesForSelect2: function() {
				return _s2Languages;
			},
			languageNamed: function(name) {
				if(typeof name === "undefined") return undefined;
				return _languageMap[name];
			},
			defaultLanguage: function() {
				return this.languageNamed(this.getPreference("defaultLanguage"));
			},
			setDefaultLanguage: function(lang) {
				this.setPreference("defaultLanguage", lang.id);
			},
			clearDefaultLanguage: function() {
				this.clearPreference("defaultLanguage");
			},
			defaultExpiration: function() {
				return this.getPreference("defaultExpiration", "-1");
			},
			setDefaultExpiration: function(value) {
				this.setPreference("defaultExpiration", value);
			},
			clearDefaultExpiration: function() {
				this.clearPreference("defaultExpiration");
			},
			setPreference: function(k, v) {
				localStorage[k] = v;
			},
			getPreference: function(k, dflt) {
				return localStorage[k] || dflt;
			},
			clearPreference: function(k) {
				delete localStorage[k];
			},
		};
	}();
})(window);

$(function() {
	"use strict";

	var pasteForm = $("#pasteForm");
	var code = $("#code"), codeeditor = $("#code-editor");
	if(pasteForm.length > 0) {
		// Initialize the form.
		var langbox = pasteForm.find("#langbox");
		var context = pasteForm.data("context");

		Ghostbin.loadLanguages();

		langbox.select2({
			data: Ghostbin.languagesForSelect2(),
			matcher: function(term, text, lang) {
				// The ifs here are blown apart so that we might short-circuit.
				if(!lang.id) return false;
				if(lang.name.toUpperCase().indexOf((''+term).toUpperCase()) >= 0) return true;
				if(lang.id.toUpperCase().indexOf((''+term).toUpperCase()) >= 0) return true;
				for(var i in lang.alt_ids) {
					if(lang.alt_ids[i].toUpperCase().indexOf((''+term).toUpperCase()) >= 0) return true;
				}
				return false;
			},
		});
		var lang = Ghostbin.languageNamed(langbox.data("selected")) ||
				Ghostbin.defaultLanguage() ||
				Ghostbin.languageNamed("text");
		langbox.select2("data", lang);

		if(context === "new") {
			pasteForm.find("input[name='expire']").val(Ghostbin.defaultExpiration());

			var optModal = $("#optionsModal");
			optModal.modal({show: false});

			optModal.find("input[type='checkbox']").on("change", function() {
				Ghostbin.setPreference($(this).data("gb-key"), this.checked ? "true" : "false");
			}).each(function() {
				this.checked = Ghostbin.getPreference($(this).data("gb-key"), "false") === "true";
			});

			$("#optionsButton").on("click", function() {
				optModal.modal("show");
			});
		}
		pasteForm.on('submit', function() {
			if((codeeditor.val().match(/[^\s]/)||[]).length !== 0) {
				if(context === "new") {
					if(Ghostbin.getPreference("saveExpiration", "false") === "true") {
						Ghostbin.setDefaultExpiration(pasteForm.find("input[name='expire']").val());
					} else {
						Ghostbin.clearDefaultExpiration();
					}

					if(Ghostbin.getPreference("saveLanguage", "false") === "true") {
						Ghostbin.setDefaultLanguage(langbox.select2("data"));
					} else {
						Ghostbin.clearDefaultLanguage();
					}
				}
			} else {
				$("#deleteModal, #emptyPasteModal").modal("show");
				return false;
			}
		});
	}

	(function(){
		var controls = $("#paste-controls");
		if(controls.length === 0) return;

		controls.onMediaQueryChanged("screen and (max-width: 767px)", function(mql) {
			this.detach();
			var newParent;
			if(mql.matches) {
				newParent = $("#phone-paste-control-container");
			} else {
				newParent = $("#desktop-paste-control-container");
			}
			newParent.prepend(this);
		});
	})();
	(function(){
		var encModal = $("#encryptModal");
		if(encModal.length === 0) return;

		encModal.modal({show: false});
		var	modalPasswordField = encModal.find("input[type='password']"),
			pastePasswordField = pasteForm.find("input[name='password']");

		modalPasswordField.keypress(function(e) {
			if(e.which === 13) {
				encModal.modal("hide");
				return false;
			}
		});

		var setEncrypted = function(encrypted) {
			$("#encryptionIcon").removeClass("icon-lock icon-lock-open-alt").addClass(encrypted ? "icon-lock" : "icon-lock-open-alt");
			$("#encryptionButton .button-data-label").text(encrypted ? "On" : "");
		};

		encModal.on("show", function() {
			modalPasswordField.val(pastePasswordField.val());
		}).on("shown", function() {
			$(this).find("input").eq(0).focus().select();
		}).on("hidden", function() {
			pastePasswordField.val(modalPasswordField.val());
			setEncrypted($(this).find("input").val().length > 0);
		});

		$("#encryptionButton").on("click", function() {
			encModal.modal("show");
		});
	})();
	(function(){
		var expModal = $("#expireModal");
		if(expModal.length === 0) return;

		expModal.modal({show: false});

		var expInput = pasteForm.find("input[name='expire']");
		var expDataLabel = $("#expirationButton .button-data-label");

		var setExpirationSelected = function() {
			$(this).button('toggle');
			expInput.val($(this).data("value"));
			expDataLabel.text($(this).data("display-value"));
		};

		setExpirationSelected.call(expModal.find("button[data-value='"+expInput.val()+"']"));
		expModal.find("button[data-value]").on("click", function() {
			setExpirationSelected.call(this);
			expModal.modal("hide");
		});

		$("#expirationButton").on("click", function() {
			expModal.modal("show");
		});
	})();

	// Common for the following functions.
	var lineNumberTrough = $("#line-numbers");

	(function(){
		if(lineNumberTrough.length === 0) return;

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
			};

			var setSelectedLineNumber = function(line) {
				if(typeof line !== 'undefined') {
					permabar.data("cur-line", line);
					history.replaceState({"line":line}, "", "#L"+line);
				} else {
					permabar.removeData("cur-line");
					history.replaceState(null, "", "#");
				}
			};

			var lineFromHash = function(hash) {
				if(!hash) return undefined;
				var v = hash.match(/^#L(\d+)/);
				if(typeof v !== 'undefined' && v.length > 0) {
					return v[1];
				}
				return undefined;
			};

			lineNumberTrough.fillWithLineNumbers((code.text().match(/\n/g)||[]).length+1, function() {
				lineNumberTrough.children().mouseenter(function() {
					positionLinebar.call(this, linebar);
				}).mouseleave(function() {
					linebar.hide();
				}).click(function() {
					var line = $(this).text();
					if((0+permabar.data("cur-line")) === line) {
						setSelectedLineNumber(undefined);
						permabar.hide();
						return;
					}
					setSelectedLineNumber(line);
					positionLinebar.call(this, permabar);
				});

				$(window).on("load popstate", function() {
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
			$(window).on("resize", function() {
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
				if(e.keyCode === 9 && !e.ctrlKey && !e.altKey && !e.shiftKey) {
					var ends = [this.selectionStart, this.selectionEnd];
					this.value = this.value.substring(0, ends[0]) + "\t" + this.value.substring(ends[1], this.value.length);
					this.selectionStart = this.selectionEnd = ends[0] + 1;
					return false;
				}
			});
		}
	})();

	$('[autofocus]:not(:focus)').eq(0).focus();
	$('[title]').tooltip({
		trigger: "hover",
		placement: "bottom",
		container: "body",
		delay: {
			show: 250,
			hide: 50,
		},
	});
	var pageLoadTime = Math.floor(new Date().getTime() / 1000);
	$('#expirationIcon').tooltip({
		trigger: "hover",
		placement: "bottom",
		container: "body",
		title: function() {
			var refTime = (0+$(this).data("reftime"));
			var curTime = Math.floor(new Date().getTime() / 1000);
			var adjust = pageLoadTime - refTime; // For the purpose of illustration, assume computer clock is faster.
			var remaining = ((0+$(this).data("value")) + adjust - curTime);
			if(remaining > 0) {
				return "Expires in " + window.Ghostbin.formatDuration(remaining);
			} else {
				var r = Math.random();
				return (r <= 0.5) ? "Wha-! It's going to explode! Get out while you still can!" : "He's dead, Jim.";
			}
		},
		delay: {
			show: 250,
			hide: 50,
		},
	});
});
