'use strict';
(async () => {
  const form = document.querySelector('#payment-form');
  if (!form) {
    if (document.querySelector('[data-payment-waiting]')) setTimeout(() => location.reload(), 5000);
    return;
  }
  const status = document.querySelector('#payment-status');
  const button = document.querySelector('#pay-button');
  const address = document.querySelector('#billing-address');
  const same = form.elements.namedItem('sameAddress');
  const toggleAddress = () => { address.hidden = same.checked; address.disabled = same.checked; };
  same.addEventListener('change', toggleAddress);
  toggleAddress();
  let card;
  try {
    if (!window.Square) throw new Error('SDK unavailable');
    const payments = window.Square.payments(form.dataset.app, form.dataset.location);
    card = await payments.card();
    await card.attach('#card-container');
    status.textContent = 'Ready for a Square test card.';
    button.disabled = false;
  } catch {
    status.textContent = 'Square’s secure card form could not load. Reload this page or return to checkout.';
    return;
  }
  let submitting = false;
  const field = (name) => String(form.elements.namedItem(name).value).trim();
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (submitting || !form.reportValidity()) return;
    submitting = true;
    button.disabled = true;
    status.textContent = 'Verifying your card with Square…';
    const billingContact = {
      givenName: field('givenName'), familyName: field('familyName'), email: field('email'),
      addressLines: same.checked ? [form.dataset.address1, form.dataset.address2].filter(Boolean) : [field('address1'), field('address2')].filter(Boolean),
      city: same.checked ? form.dataset.city : field('city'),
      postalCode: same.checked ? form.dataset.postcode : field('postalCode'),
      countryCode: same.checked ? form.dataset.country : field('countryCode').toUpperCase()
    };
    let result;
    try {
      result = await card.tokenize({amount: form.dataset.amount, currencyCode: 'GBP', intent: 'CHARGE', customerInitiated: true, sellerKeyedIn: false, billingContact});
    } catch {
      status.textContent = 'Card verification was cancelled or could not finish. No payment request was sent. You can try again.';
      submitting = false; button.disabled = false; return;
    }
    if (result.status !== 'OK' || !result.token) {
      status.textContent = 'Square could not verify this card. No payment request was sent. Check the test card and try again.';
      submitting = false; button.disabled = false; return;
    }
    status.textContent = 'Confirming your test payment. Please keep this page open…';
    try {
      const response = await fetch(location.pathname, {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json', 'X-Selloovy-Request': 'checkout-payment'}, body: JSON.stringify({csrf: form.dataset.csrf, revision: Number(form.dataset.revision), source: result.token})});
      if (!response.ok) {
        const reply = await response.json().catch(() => ({}));
        status.textContent = reply.error || 'Payment could not be confirmed. Reload its status before trying again.';
        const link = document.createElement('a'); link.href = location.pathname; link.textContent = 'Reload payment status'; status.append(' ', link);
        return;
      }
      location.reload();
    } catch {
      status.textContent = 'The connection was interrupted. Reload payment status before trying again; do not start another payment.';
      const link = document.createElement('a'); link.href = location.pathname; link.textContent = 'Reload payment status'; status.append(' ', link);
    }
  });
})();
