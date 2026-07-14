export default {
  async fetch(request, env) {
    return new Response("<html>cat</html>", {
      headers: { "content-type": "text/html" },
    });
  },
};
