// @ts-check

/** Format validated exact USD without converting its integers to Number.
 * @param {import('../types.d.js').ExactMoney} amount
 */
export function formatExactUSD(amount) {
  const numerator=BigInt(amount.numerator), denominator=BigInt(amount.denominator);
  const scale=1000000n, scaled=numerator*scale;
  const units=scaled/denominator, remainder=scaled%denominator;
  if (units===0n && numerator>0n) return 'Less than $0.000001';
  const digits=String(units%scale).padStart(6,'0').replace(/0{1,4}$/,'');
  return `${remainder?'≈':''}$${units/scale}.${digits}`;
}
