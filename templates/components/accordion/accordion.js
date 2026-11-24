// Toggle accordion logic delegating to the CSP-safe GoStack runtime method
function toggleAccordion(trigger) {
  if (typeof window !== 'undefined' && window.GoStack && window.GoStack.toggleAccordion) {
    window.GoStack.toggleAccordion(trigger);
  }
}