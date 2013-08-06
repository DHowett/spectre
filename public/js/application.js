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
				n += ""+(i+1)+"\n";
			}
			ln.text(n);

			curLines = lines;
			if(callback) callback();
		}

		if($("#code").length > 0) {
			fillForLines(($("#code").text().match(/\n/g)||[]).length);
		} else if($("#code-editor").length > 0) {
			var updateLines = function() {
				fillForLines(($(this).val().match(/\n/g)||[]).length + 1, function() {
					$(".textarea-height-wrapper").css("left", ln.outerWidth());
					$(".textarea-height-wrapper").css("height", ln.outerHeight());
					$(".textarea-world-wrapper").css("height", ln.outerHeight());
				});
			}
			$("#code-editor").on("input propertychange", updateLines);
			updateLines.call($("#code-editor").get(0));
		}
	})();
	(function(){
		$('[autofocus]:not(:focus)').eq(0).focus();
	})();
	(function(){
		$("#code-editor").keydown(function(e) {
			if(e.keyCode == 9) {
				var ends = [this.selectionStart, this.selectionEnd];
				this.value = this.value.substring(0, ends[0]) + "\t" + this.value.substring(ends[1], this.value.length);
				this.selectionStart = this.selectionEnd = ends[0] + 1;
				return false;
			}
		});
	})();
});
