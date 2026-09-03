import { chromium } from 'playwright';
const b = await chromium.launch(); const pg = await b.newPage();
const errs = [];
pg.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') errs.push(m.type()+': '+m.text()); });
pg.on('pageerror', e => errs.push('PAGEERROR: ' + e.message));
await pg.goto('http://localhost:5173/design/f1-aurora.html');
await pg.waitForTimeout(800);
console.log('ideas=', await pg.textContent('#statIdeas'));
// edit click
const editBtn = pg.locator('li.item:not(.is-removed) .act.edit').first();
await editBtn.click({ force: true });
await pg.waitForTimeout(300);
console.log('dialog open=', await pg.evaluate(() => document.getElementById('editDlg').open));
console.log('errs so far:', errs.slice(0,6));
// notes toggle in composer
await pg.locator('#noteToggle').click();
console.log('extra open=', await pg.evaluate(() => document.getElementById('extraWrap').classList.contains('open')));
// removed toggle
const rt = pg.locator('#removedToggle');
console.log('rt hidden=', await rt.isHidden(), 'count=', await pg.textContent('#removedCount').catch(()=>'-'));
if (await rt.isVisible()) { await rt.click(); await pg.waitForTimeout(500);
  console.log('removed visible count=', await pg.locator('li.item.is-removed').count()); }
console.log(errs.join('\n') || 'NO ERRORS');
await b.close();
