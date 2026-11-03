// Fills the sidebar's Z-Library quota line from /zlib/quota. Static script
// (loaded once, never re-run by a Drive morph): the sidebar node keeps its id
// across morphs, so listeners survive and a refresh re-fills the same span.
(function () {
  function fill() {
    var box = document.getElementById("zlib-quota");
    if (!box || box.dataset.loading === "1") return;
    box.dataset.loading = "1";
    fetch(box.dataset.endpoint, { credentials: "same-origin", headers: { Accept: "application/json" } })
      .then(function (response) {
        return response.ok ? response.json() : null;
      })
      .then(function (data) {
        var slot = box.querySelector("[data-zlib-quota-fill]");
        var reset = box.querySelector("[data-zlib-quota-reset]");
        if (!data || data.enabled !== true || data.unavailable === true || !data.limit) {
          box.hidden = true;
          return;
        }
        if (slot) slot.textContent = data.used + "/" + data.limit;
        if (reset) {
          var left = resetLabel(box.dataset.resetLabel, data.resetAt);
          reset.textContent = left;
          reset.hidden = left === "";
        }
        box.hidden = false;
      })
      .catch(function () {
        box.hidden = true;
      })
      .then(function () {
        delete box.dataset.loading;
      });
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", fill);
  } else {
    fill();
  }
  document.addEventListener("amarra:morphed", fill);
})();

// resetLabel turns the ISO reset instant into "renova em 7h23m" (localized
// prefix from the template); an empty string hides the line.
function resetLabel(prefix, resetAt) {
  if (!prefix || !resetAt) return "";
  var until = new Date(resetAt).getTime() - Date.now();
  if (!isFinite(until) || until <= 0) return "";
  var minutes = Math.floor(until / 60000);
  var hours = Math.floor(minutes / 60);
  var rest = minutes % 60;
  if (hours <= 0) return prefix + " " + rest + "m";
  return prefix + " " + hours + "h" + String(rest).padStart(2, "0");
}
