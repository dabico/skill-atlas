(function () {
  var box = document.getElementById("filter");
  var q = document.getElementById("q");
  var count = document.getElementById("fcount");
  var none = document.getElementById("nomatch");
  var items = document.querySelectorAll("[data-match]");
  var total = document.querySelectorAll("section[data-match]").length;

  // Same rule as the TUI: case-insensitive substring of name, description or path.
  function apply() {
    var f = q.value.toLowerCase();
    var shown = 0;
    for (var i = 0; i < items.length; i++) {
      var hit = f === "" || items[i].dataset.match.toLowerCase().indexOf(f) !== -1;
      items[i].hidden = !hit;
      if (hit && items[i].tagName === "SECTION") shown++;
    }
    count.hidden = f === "";
    count.textContent = "Skills " + shown + "/" + total;
    none.hidden = f === "" || shown !== 0;
  }

  function clear() {
    q.value = "";
    apply();
  }

  q.addEventListener("input", apply);
  document.addEventListener("keydown", function (e) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "Escape" && (q.value !== "" || document.activeElement === q)) {
      clear();
      q.blur();
    } else if (e.key === "/") {
      var a = document.activeElement;
      var typing = a && (a.tagName === "INPUT" || a.tagName === "TEXTAREA" || a.tagName === "SELECT" || a.isContentEditable);
      if (!typing) {
        e.preventDefault();
        q.focus();
      }
    }
  });

  box.hidden = false;
  apply();
})();
