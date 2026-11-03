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
        if (!data || data.enabled !== true || data.unavailable === true || !data.limit) {
          box.hidden = true;
          return;
        }
        if (slot) slot.textContent = data.used + "/" + data.limit;
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
