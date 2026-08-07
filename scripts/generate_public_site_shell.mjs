// @ts-check

import { readFile, writeFile } from "node:fs/promises";
import { renderPublicFooter, renderPublicHeader } from "./public_site_shell.mjs";

const LANDING_PATH = "site/index.html";
const CHECK_ARGUMENT = "--check";
const HEADER_START = "    <!-- llm-proxy-public-header:start -->";
const HEADER_END = "    <!-- llm-proxy-public-header:end -->";
const FOOTER_START = "    <!-- llm-proxy-public-footer:start -->";
const FOOTER_END = "    <!-- llm-proxy-public-footer:end -->";

const unexpectedArguments = process.argv.slice(2).filter((argument) => argument !== CHECK_ARGUMENT);
if (unexpectedArguments.length > 0) {
  throw new Error(`public_site_shell_unknown_argument: ${unexpectedArguments.join(",")}`);
}

const source = await readFile(LANDING_PATH, "utf8");
const rendered = replaceFragment(
  replaceFragment(source, HEADER_START, HEADER_END, renderPublicHeader()),
  FOOTER_START,
  FOOTER_END,
  renderPublicFooter(),
);

if (process.argv.includes(CHECK_ARGUMENT)) {
  if (source !== rendered) {
    throw new Error(`public_site_shell_out_of_date: run node scripts/generate_public_site_shell.mjs`);
  }
  console.log(`verified shared public shell in ${LANDING_PATH}`);
} else {
  await writeFile(LANDING_PATH, rendered, "utf8");
  console.log(`generated shared public shell in ${LANDING_PATH}`);
}

/**
 * @param {string} sourceDocument
 * @param {string} startMarker
 * @param {string} endMarker
 * @param {string} fragment
 * @returns {string}
 */
function replaceFragment(sourceDocument, startMarker, endMarker, fragment) {
  const startIndex = sourceDocument.indexOf(startMarker);
  const endIndex = sourceDocument.indexOf(endMarker);
  if (
    startIndex === -1 ||
    endIndex === -1 ||
    startIndex !== sourceDocument.lastIndexOf(startMarker) ||
    endIndex !== sourceDocument.lastIndexOf(endMarker) ||
    endIndex <= startIndex
  ) {
    throw new Error(`public_site_shell_markers_invalid: start=${startMarker} end=${endMarker}`);
  }
  return `${sourceDocument.slice(0, startIndex)}${startMarker}\n${fragment}\n${endMarker}${sourceDocument.slice(endIndex + endMarker.length)}`;
}
