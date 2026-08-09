import esbuild from "esbuild";

const target = process.argv[2]; // "js", "css", or undefined (both)

if (!target || target === "js") {
  esbuild.buildSync({
    entryPoints: ["index.ts"],
    bundle: true,
    minify: true,
    format: "iife",
    outfile: "../app/static/bundle.js",
    target: ["es2020"],
    logLevel: "info",
  });
}

if (!target || target === "css") {
  esbuild.buildSync({
    entryPoints: ["../css/style.css"],
    bundle: true,
    minify: true,
    external: ["*.woff2"],
    outfile: "../app/static/style.min.css",
    target: ["chrome100", "firefox100", "safari16"],
    logLevel: "info",
  });
}
