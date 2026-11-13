/** Applies the saved theme and privacy mode before React loads (RNF-14, RNF-15). */
(function () {
  try {
    var theme = localStorage.getItem("radar.theme") || "system";
    var dark = theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
    document.documentElement.classList.toggle("dark", dark);
    // A single theme color with the chosen theme, for the system bar of Android (RNF-23).
    var metas = document.querySelectorAll('meta[name="theme-color"]');
    for (var i = 1; i < metas.length; i++) metas[i].remove();
    if (metas[0]) {
      metas[0].removeAttribute("media");
      metas[0].content = dark ? "#0b0d12" : "#f4f5f7";
    }
    if (localStorage.getItem("radar.privacy") === "1") {
      document.documentElement.classList.add("privacy");
    }
  } catch (e) {
    /* Storage unavailable: keep the defaults. */
  }
})();
