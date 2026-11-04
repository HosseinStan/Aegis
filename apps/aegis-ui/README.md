# Aegis UI integration

Aegis does not replace the existing Vue.js office application. Integration
examples are maintained in [`docs/vue-integration.md`](../../docs/vue-integration.md).

Explore the [standalone MVP preview](preview/README.md) with synthetic demo data.

The additional [Vue MVP slice](vue-mvp/README.md) implements Dashboard,
Inventory, basic device profiles, Saved Configuration, Backup History, native Configuration Compare and Change Review against the same isolated Go demo. It is
an additional development frontend while the full standalone preview retains
Saved Configuration, Backup History, native Compare and Change Review.

The Vue slice also provides a [one-command Windows launcher](vue-mvp/README.md#one-command-windows-workspace)
for the existing isolated Go/Node/Vue services, with no automatic dependency installation.

