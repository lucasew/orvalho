import { greet } from "./lib/util.js";
export default { async fetch() { return new Response(greet()); } };
