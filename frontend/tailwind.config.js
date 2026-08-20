/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["../views/**/*.html"],
  plugins: [require("daisyui"), require("@tailwindcss/typography")],
  daisyui: {
    themes: ["nord", "night"],
  },
}
