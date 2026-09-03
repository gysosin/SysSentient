/** @type {import('tailwindcss').Config} */
//
// This file exists for one reason: Tailwind v4 needs an explicit `@config`
// directive to load it, and `web/index.css` declares one. The design system
// itself lives in that stylesheet as CSS custom properties mapped through
// `@theme inline` — that is the single source of truth for colour, radius and
// type.
//
// A `theme.extend` block used to live here too, carrying a second, contradictory
// palette (GitHub-dark surfaces, a blue accent, Inter). Nothing in the app ever
// referenced it, and once the real tokens moved into index.css it was actively
// misleading: two files claiming to define the same thing, one of them dead.
// Keep this file limited to build inputs.
export default {
  content: [
    "./index.html",
    "./**/*.{js,ts,jsx,tsx}",
    // The unbounded glob above would otherwise crawl node_modules (and build
    // output) on every build now that @config actually loads.
    "!./node_modules/**",
    "!./dist/**",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
