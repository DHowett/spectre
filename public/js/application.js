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
		$('[autofocus]:not(:focus)').eq(0).focus();
	})();
});
