// Canonical bootstrap source. Regenerate the Desktop public asset after changes;
// the shared preference tests verify that the shipped script stays identical.
export const UI_THEME_BOOTSTRAP = 'try{if(localStorage.getItem("multica-ui-theme")==="classic")document.documentElement.setAttribute("data-ui-theme","classic")}catch{/* Device storage may be unavailable. */}';

/** Inline in the document head so classic is restored before any body paints. */
export function UiThemeScript() {
  return <script dangerouslySetInnerHTML={{ __html: UI_THEME_BOOTSTRAP }} />;
}
