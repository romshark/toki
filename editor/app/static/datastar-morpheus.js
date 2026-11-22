// The page loads Datastar through this module, which defines the Morpheus
// elements first. Datastar applies its attributes in a timer it sets when it
// loads. A data-bind it applies to a custom element not yet defined binds the
// value attribute instead of the value property, and <neo-textinput> never
// updates that attribute. Imports evaluate in order, with no task between
// them in which that timer could run.
import "./morpheus.js";
export * from "./datastar.js";
