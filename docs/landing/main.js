document.documentElement.classList.replace('no-js', 'js');

// Scroll-reveal: fade sections in as they enter the viewport.
(function () {
  const els = document.querySelectorAll('.reveal');
  if (!('IntersectionObserver' in window)) {
    els.forEach((el) => el.classList.add('in'));
    return;
  }
  const io = new IntersectionObserver(
    (entries) => {
      entries.forEach((e) => {
        if (e.isIntersecting) {
          e.target.classList.add('in');
          io.unobserve(e.target);
        }
      });
    },
    { threshold: 0.12, rootMargin: '0px 0px -8% 0px' }
  );
  els.forEach((el) => io.observe(el));
})();

// Copy-to-clipboard for install commands.
const copyStatus = document.getElementById('copy-status');

async function copyText(text) {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (_) {}
  }

  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.className = 'copy-helper';
  document.body.appendChild(ta);
  ta.select();
  let copied = false;
  try { copied = document.execCommand('copy'); } catch (_) {}
  document.body.removeChild(ta);
  return copied;
}

document.querySelectorAll('.copy').forEach((btn) => {
  btn.addEventListener('click', async () => {
    const text = btn.getAttribute('data-copy') || '';
    const label = btn.getAttribute('data-copy-label') || 'command';
    const copied = await copyText(text);
    const original = btn.textContent;
    btn.textContent = copied ? 'Copied' : 'Copy failed';
    btn.classList.toggle('done', copied);
    if (copyStatus) copyStatus.textContent = copied ? `${label} copied.` : `Could not copy ${label}. Select the command manually.`;
    setTimeout(() => {
      btn.textContent = original;
      btn.classList.remove('done');
    }, 1500);
  });
});
