/** Applies the saved theme and privacy mode before React loads (RNF-14, RNF-15). */
(function () {
  try {
    var theme = localStorage.getItem("radar.theme") || "system";
    var dark = theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
    document.documentElement.classList.toggle("dark", dark);
    if (localStorage.getItem("radar.privacy") === "1") {
      document.documentElement.classList.add("privacy");
    }
  } catch (e) {
    /* Storage unavailable: keep the defaults. */
  }
})();
