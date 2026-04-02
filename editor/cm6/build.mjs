import esbuild from "esbuild";

esbuild.buildSync({
  entryPoints: ["index.js"],
  bundle: true,
  minify: true,
  format: "iife",
  outfile: "../app/static/codemirror6.min.js",
  target: ["es2020"],
});

console.log("Built codemirror6.min.js");
