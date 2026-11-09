// The suites say yes to every question. A question is either the browser's own dialog or, where
// hel.ink serves hop-ui.js, the HOP family's shared dialog: a <dialog> in the page. This accepts both.
//   require('./e2e-dialogs')(page);
module.exports = function confirmDialogs(page) {
  page.on('dialog', d => d.accept().catch(() => {}));
  page.addInitScript(() => {
    new MutationObserver(() => {
      document.querySelectorAll('dialog.hop-dialog:not([data-e2e])').forEach(d => {
        d.dataset.e2e = '1';
        setTimeout(() => { const ok = d.querySelector('button[type=submit]'); if (ok) ok.click(); }, 40);
      });
    }).observe(document, { childList: true, subtree: true });
  }).catch(() => {});
};
