# The dev shell's sandbox: the whole system read-only, but for the
# checkout, the caches and /tmp. With no command it runs the user's
# shell; <name>-sandbox make test runs the suite in it.
{ writeShellApplication, bubblewrap, name }:

writeShellApplication {
  name = "${name}-sandbox";
  runtimeInputs = [ bubblewrap ];
  text = ''
    [ $# -gt 0 ] || set -- "$SHELL" -i
    exec bwrap \
      --ro-bind / / \
      --unshare-pid \
      --bind "$PWD" "$PWD" \
      --bind "$HOME/.cache" "$HOME/.cache" \
      --bind /tmp /tmp \
      --dev-bind /dev /dev \
      --proc /proc \
      --die-with-parent \
      --setenv IN_SANDBOXED_SHELL 1 \
      -- "$@"
  '';
}
