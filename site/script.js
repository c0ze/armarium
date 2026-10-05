// The content and links work without JavaScript. Enhance the setup switcher and
// copy controls only after the page has loaded.
const tabs = [...document.querySelectorAll('.setup-tab')];
const panels = tabs.map((tab) => document.getElementById(tab.dataset.panel));
const tablist = document.querySelector('.setup-tabs');

function selectTab(index, focus = false) {
  tabs.forEach((tab, i) => {
    const selected = i === index;
    tab.classList.toggle('active', selected);
    tab.setAttribute('aria-selected', String(selected));
    tab.tabIndex = selected ? 0 : -1;
    panels[i].hidden = !selected;
  });
  if (focus) tabs[index].focus();
}

tablist.setAttribute('role', 'tablist');
tabs.forEach((tab, index) => {
  tab.setAttribute('role', 'tab');
  tab.setAttribute('aria-controls', tab.dataset.panel);
  panels[index].setAttribute('role', 'tabpanel');
  panels[index].tabIndex = 0;
  tab.addEventListener('click', () => selectTab(index));
  tab.addEventListener('keydown', (event) => {
    let next;
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
    if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
    if (event.key === 'Home') next = 0;
    if (event.key === 'End') next = tabs.length - 1;
    if (next !== undefined) {
      event.preventDefault();
      selectTab(next, true);
    }
  });
});
selectTab(0);

const status = document.querySelector('.copy-status');
let statusTimer;
document.querySelectorAll('[data-copy]').forEach((button) => {
  if (!navigator.clipboard?.writeText) return;
  button.hidden = false;
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(document.getElementById(button.dataset.copy).textContent);
      status.textContent = 'Copied to clipboard.';
    } catch {
      status.textContent = 'Copy unavailable. Select the command and copy it manually.';
    }
    clearTimeout(statusTimer);
    statusTimer = setTimeout(() => { status.textContent = ''; }, 3500);
  });
});
