
const filter = contents => {
  if (contents.type !== 'com') return;
  const t = Math.floor(contents.time_us / 1000);
  const { type: commitType, rkey, record } = contents.commit;
  if (['c', 'u'].includes(commitType)) {
    const { text, langs } = record;
    let type;
    if (commitType === 'c') {
      if (text === '') return;
      type = 'post';
    } else if (commitType === 'u') {
      if (text === '') return { type: 'delete', t, rkey };
      type = 'update';
    }
    const normLangs = langs?.map(lang => lang.split('-')[0]);
    return { type, t, rkey, langs: normLangs, text };
  } else if (commitType === 'd') {
    return { type: 'delete', t, rkey };
  }
}

export default filter;