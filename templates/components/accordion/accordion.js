// Toggle accordion logic
function toggleAccordion(trigger) {
  const item = trigger.parentElement;
  const content = item.querySelector('.gostack-components-accordion-content');
  
  if (item.classList.contains('active')) {
    item.classList.remove('active');
    content.style.maxHeight = '0px';
  } else {
    document.querySelectorAll('.gostack-components-accordion-item').forEach(el => {
      el.classList.remove('active');
      el.querySelector('.gostack-components-accordion-content').style.maxHeight = '0px';
    });
    item.classList.add('active');
    content.style.maxHeight = '200px';
  }
}