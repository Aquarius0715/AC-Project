// Embedded into offline HTML by html_report.py; no network or storage writes.
function renderProposalCards(day, parent) {
  const panel = el('article', undefined, 'panel');
  panel.id = 'proposal-cards';
  panel.append(el('h2', '有効な提案カード'), el('p', '曜日別の根拠で判定し、同じ機器・同じ表示開始の平日と土日をまとめます。チェックは承認対象のプレビューです。入力JSONの反応や予定は書き換えません。', 'muted'));
  if (!day.cards?.length) panel.append(el('p', '現在有効なカードはありません。根拠不足・固定・承認・見送りなどの理由は下の曜日別判定を確認してください。'));
  (day.cards || []).forEach(card => {
    const article = el('section', undefined, 'proposal-card');
    article.dataset.cardId = card.card_id;
    article.append(el('h3', card.title), el('p', card.device_id + ' / カード ' + card.card_id, 'muted'));
    if (card.kind !== 'start') {
      card.members.forEach(m => article.append(el('p', m.proposal_id + ' / ' + label(m.kind) + ' / ' + JSON.stringify(m.values))));
      detail(article, '根拠と対象の詳細', card.members);
      panel.append(article);
      return;
    }
    const controls = [], t = el('table'), head = el('tr');
    ['承認対象', '曜日', '現行開始 → 表示開始', '維持する終了・温度', '候補の支持日数・支持日', '曜日別提案ID'].forEach(x => head.append(el('th', x)));
    t.append(head);
    card.members.forEach(member => {
      const row = el('tr'), cell = el('td'), checkbox = el('input');
      checkbox.type = 'checkbox';
      checkbox.checked = true;
      checkbox.setAttribute('aria-label', WEEKDAY_NAMES[member.weekday] + '曜を承認対象にする');
      checkbox.dataset.proposalId = member.proposal_id;
      const wrapper = el('label'); wrapper.append(checkbox, el('span', ' 対象')); cell.append(wrapper);
      row.append(cell, el('td', WEEKDAY_NAMES[member.weekday] + '曜'),
        el('td', member.current.start + ' → ' + member.values.start),
        el('td', member.values.end + ' / ' + member.values.temperature_c + '℃'),
        el('td', member.support_count + '日 / ' + (member.support_dates || []).join('・')),
        el('td', member.proposal_id));
      t.append(row);
      controls.push({ checkbox, member });
    });
    article.append(t);
    const preview = el('div', undefined, 'notice approval-preview');
    preview.setAttribute('aria-live', 'polite');
    const update = () => {
      preview.replaceChildren();
      const selected = controls.filter(c => c.checkbox.checked).map(c => c.member);
      const excluded = controls.filter(c => !c.checkbox.checked).map(c => c.member);
      preview.append(el('strong', '今回の承認対象：' + (selected.map(m => WEEKDAY_NAMES[m.weekday]).join('・') || 'なし') + '（' + selected.length + '曜日）'));
      if (selected.length) table(preview, ['曜日', '適用される開始', '維持する終了・温度', '参照する提案ID'], selected.map(m => [WEEKDAY_NAMES[m.weekday], m.values.start, m.values.end + ' / ' + m.values.temperature_c + '℃', m.proposal_id]));
      preview.append(el('p', excluded.length ? excluded.map(m => WEEKDAY_NAMES[m.weekday]).join('・') + '曜は今回だけ対象外です。固定や恒久的な拒否にはなりません。' : '外した曜日は今回の承認に含めません。固定・解除は別の明示的な設定です。'));
    };
    controls.forEach(c => { c.checkbox.onchange = update; });
    article.append(preview); update();
    detail(article, '曜日別の表示値・日々の候補・根拠署名', card.members);
    panel.append(article);
  });
  parent.append(panel);
}

const WEEKDAY_NAMES = '月火水木金土日';
function renderDelivery(day, parent) {
  const panel = el('article', undefined, 'panel');
  panel.id = 'delivery';
  panel.append(el('h2', '通知予定と曜日別の反応'));
  const notice = day.notification;
  if (notice) {
    panel.append(el('p', (notice.scheduled ? '通知予定あり：' + notice.scheduled_at : '通知予定なし') + ' / 週の開始 ' + notice.week_start, 'notice'), el('p', notice.reason));
    const rows = members => members.map(m => [m.device_id, label(m.kind), m.weekday === null ? '共通' : WEEKDAY_NAMES[m.weekday], m.values, m.proposal_id, m.first_waiting_at]);
    if (notice.targets.length) table(panel, ['機器', '種類', '曜日', '通知する内容', '曜日別ID', '確認待ち開始'], rows(notice.targets));
    if (notice.pending.length) {
      panel.append(el('h3', '有効な未通知案（翌週へ持ち越し）'));
      table(panel, ['機器', '種類', '曜日', '内容', '曜日別ID', '確認待ち開始'], rows(notice.pending));
    }
    if (notice.dropped.length) detail(panel, '有効でなくなったため通知待ちから除外', notice.dropped);
  }
  if (day.responses.length) {
    panel.append(el('h3', '入力した反応の再生結果'));
    table(panel, ['反応日時', '曜日', '判断', '参照カード / 提案ID', '表示していた内容'], day.responses.map(r => [r.responded_at, r.weekday === null ? '共通' : WEEKDAY_NAMES[r.weekday], label(r.response), (r.card_id || '') + ' / ' + (r.proposal_id || r.id), r.values]));
  }
  if (day.approval_targets?.length) {
    panel.append(el('h3', '今回承認した曜日だけの変更対象'));
    table(panel, ['機器', '曜日', '承認した変更', '参照ID'], day.approval_targets.map(m => [m.device_id, m.weekday === null ? '共通' : WEEKDAY_NAMES[m.weekday], m.values, m.proposal_id || '従来履歴']));
    panel.append(el('p', '現在の予定への反映は入力の有効な予定版で確認します。この画面から実機や予定への適用は行いません。', 'muted'));
  }
  if (day.start_controls?.length) detail(panel, 'この判定までに受信した固定・解除設定', day.start_controls);
  parent.append(panel);
}
