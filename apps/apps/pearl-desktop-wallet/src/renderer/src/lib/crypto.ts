function formatTxid(txid: string) {
  if (txid.length <= 16) {
    return txid;
  }
  return `${txid.slice(0, 8)}...${txid.slice(-8)}`;
}

function formatMDLAmount(amount: number | string) {
  const amountStr = String(amount);

  const decimalIndex = amountStr.indexOf('.');
  if (decimalIndex === -1) {
    return amountStr;
  }

  const decimalPlaces = amountStr.length - decimalIndex - 1;

  if (decimalPlaces <= 8) {
    return amountStr;
  }

  // If more than 8 decimal places, truncate to 8 and remove trailing zeros
  return (Math.floor(Number(amount) * 1e8) / 1e8).toFixed(8).replace(/\.?0+$/, '');
}

export { formatTxid, formatMDLAmount };
