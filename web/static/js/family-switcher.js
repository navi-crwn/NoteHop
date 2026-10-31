document.addEventListener('keydown', event => {
  document.querySelectorAll('[data-family-switcher][open]').forEach(menu => {
    if (event.key === 'Escape') {
      menu.open = false;
      menu.querySelector('summary').focus();
      event.preventDefault();
    } else if (menu.contains(event.target) && ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      const links = [...menu.querySelectorAll('a')];
      let i = links.indexOf(document.activeElement);
      i = event.key === 'Home' ? 0 : event.key === 'End' ? links.length - 1 : event.key === 'ArrowDown' ? (i + 1) % links.length : (i <= 0 ? links.length : i) - 1;
      links[i]?.focus();
      event.preventDefault();
    }
  });
});
document.addEventListener('click', event => {
  document.querySelectorAll('[data-family-switcher][open]').forEach(menu => {
    if (!menu.contains(event.target)) menu.open = false;
  });
});
