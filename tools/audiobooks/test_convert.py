import pathlib, tempfile, unittest, json, subprocess, sys
from audit import probe
from convert import convert, run, sha256

class ConversionTest(unittest.TestCase):
    def test_preserves_audio_chapters_cover_and_original(self):
        with tempfile.TemporaryDirectory() as directory:
            base=pathlib.Path(directory).resolve();root=base/'source';root.mkdir();book=root/'Author'/'Book';book.mkdir(parents=True)
            source=book/'Book.m4b';image=base/'cover.jpg';meta=base/'chapters.txt'
            meta.write_text(';FFMETADATA1\ntitle=Test Book\nartist=Test Author\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1500\ntitle=One\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1500\nEND=3000\ntitle=Two\n')
            run(['ffmpeg','-v','error','-f','lavfi','-i','color=c=blue:s=200x200','-frames:v','1',str(image)])
            run(['ffmpeg','-v','error','-f','lavfi','-i','sine=frequency=440:duration=3','-i',str(meta),'-map','0:a:0','-map_metadata','1','-map_chapters','1','-c:a','aac','-f','mp4',str(source)])
            original=sha256(source)
            with self.assertRaisesRegex(ValueError,'Cover required'):convert(source,root,base/'out',32)
            with self.assertRaisesRegex(ValueError,'outside'):convert(source,root,root/'out',32,image)
            receipt=convert(source,root,base/'out',32,image)
            result=pathlib.Path(receipt['output']);self.assertEqual(result.relative_to(base/'out'),pathlib.Path('Author/Book/Book.m4b'))
            self.assertEqual(original,sha256(source));self.assertEqual(receipt['chapters'],2)
            self.assertAlmostEqual(float(probe(result)['format']['duration']),3,delta=.1)
            with self.assertRaises(FileExistsError):convert(source,root,base/'out',32,image)
            # Reuse embedded artwork and remux chapters from a covered AAC file.
            covered=book/'Covered.m4b'
            run(['ffmpeg','-v','error','-i',str(source),'-i',str(image),'-map','0:a:0','-map','1:v:0','-map_metadata','0','-map_chapters','0','-c','copy','-disposition:v','attached_pic','-f','mp4',str(covered)])
            self.assertTrue(convert(covered,root,base/'embedded',32)['embedded_cover'])
            video=book/'Video.m4b'
            run(['ffmpeg','-v','error','-i',str(source),'-f','lavfi','-i','color=c=black:s=64x64:d=3','-map','0:a:0','-map','1:v:0','-c:a','copy','-c:v','mpeg4','-shortest','-f','mp4',str(video)])
            with self.assertRaisesRegex(ValueError,'Non-cover video'):convert(video,root,base/'video',32,image)
            manifest=base/'plan.json';batch_output=base/'batch'
            manifest.write_text(json.dumps({'jobs':[{'action':'pilot_candidate','source':str(covered),'conversion_root':str(root),'target_kbps':32,'source_bytes':covered.stat().st_size,'estimated_output_bytes':12000,'embedded_cover':True}]}))
            command=[sys.executable,str(pathlib.Path(__file__).with_name('batch.py')),str(manifest),'--output-root',str(batch_output)]
            run(command);self.assertFalse(batch_output.exists())
            run(command+['--execute']);result=run(command+['--execute']);self.assertIn('"selected": 0',result.stdout)
            staged=batch_output/'Author/Book/Covered.m4b'
            with staged.open('ab') as f:f.write(b'corruption')
            self.assertNotEqual(subprocess.run(command+['--execute'],capture_output=True).returncode,0)
if __name__=='__main__':unittest.main()
