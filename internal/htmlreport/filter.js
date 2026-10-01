(function () {
  var box = document.getElementById("filter");
  var q = document.getElementById("q");
  var count = document.getElementById("fcount");
  var none = document.getElementById("nomatch");
  var items = document.querySelectorAll("[data-match]");
  var total = document.querySelectorAll("section[data-match]").length;
  var groups = document.querySelectorAll("[data-group]");
  var sort = document.getElementById("sort");

  // Same rule as the TUI: case-insensitive substring of name, description, path or repository.
  function apply() {
    var f = q.value.toLowerCase();
    var shown = 0;
    for (var i = 0; i < items.length; i++) {
      var hit = f === "" || items[i].dataset.match.toLowerCase().indexOf(f) !== -1;
      items[i].hidden = !hit;
      if (hit && items[i].tagName === "SECTION") shown++;
    }
    // A repository group shows only while one of its skills does.
    for (var g = 0; g < groups.length; g++) {
      groups[g].hidden = f !== "" && groups[g].querySelector("[data-match]:not([hidden])") === null;
    }
    count.hidden = f === "";
    count.textContent = "Skills " + shown + "/" + total;
    none.hidden = f === "" || shown !== 0;
  }

  // The page comes in name order A–Z. Z–A is that order reversed: the contents entries, the skill
  // sections and the repository groups each flip in place. The filter state moves with the nodes.
  var runs = [];
  function collect(nodes) {
    for (var i = 0; i < nodes.length; i++) {
      var run = null;
      for (var r = 0; r < runs.length; r++) {
        if (runs[r].parent === nodes[i].parentNode) run = runs[r];
      }
      if (run === null) {
        run = { parent: nodes[i].parentNode, nodes: [] };
        runs.push(run);
      }
      run.nodes.push(nodes[i]);
    }
  }
  collect(items);
  collect(groups);

  var desc = false;
  function order() {
    if ((sort.value === "desc") === desc) return;
    desc = !desc;
    for (var r = 0; r < runs.length; r++) {
      var n = runs[r].nodes;
      var after = n[n.length - 1].nextSibling;
      for (var i = n.length - 1; i >= 0; i--) runs[r].parent.insertBefore(n[i], after);
      n.reverse();
    }
  }

  function clear() {
    q.value = "";
    apply();
  }

  q.addEventListener("input", apply);
  sort.addEventListener("change", order);
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
  order();
  apply();
})();
