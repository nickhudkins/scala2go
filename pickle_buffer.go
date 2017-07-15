package main

import (
	"fmt"
	"io"
)

type pickleProcessor struct {
	ix      []int32
	visited []string
	r       io.ReadSeeker
}

func readByte(r io.ReadSeeker, buf []byte) (byte, error) {
	_, err := io.ReadFull(r, buf[:1])
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readLongNat(r io.ReadSeeker, buf []byte) (int64, error) {
	var x int64
	for {
		b, err := readByte(r, buf)
		if err != nil {
			return 0, err
		}
		x = (x << 7) + (int64(b) & 0x7f)
		if (b & 0x80) == 0 {
			break
		}
	}
	return x, nil
}

func createIndex(r io.ReadSeeker, buf []byte) ([]int32, error) {
	length, err := readLongNat(r, buf)
	if err != nil {
		return nil, err
	}
	index := make([]int32, length)
	for i := int64(0); i < length; i++ {
		readIndex, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		index[i] = int32(readIndex)
		//discard
		_, err = readByte(r, buf)
		if err != nil {
			return nil, err
		}
		offset, err := readLongNat(r, buf)
		if err != nil {
			return nil, err
		}
		r.Seek(offset, io.SeekCurrent)
	}
	return index, nil
}

func (pp *pickleProcessor) readProcessRef(i int, name string, buf []byte) error {
	ref, err := readLongNat(pp.r, buf)
	if err != nil {
		return err
	}
	return pp.processRef(i, int(ref), name, buf)
}

func (pp *pickleProcessor) processRef(i int, refI int, name string, buf []byte) error {
	fmt.Println("processing", name, i, refI)
	pos, err := pp.r.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	err = pp.readProcessEntry(refI, buf)
	if err != nil {
		return err
	}
	_, err = pp.r.Seek(pos, io.SeekStart)
	if err != nil {
		return err
	}
	return nil
}

func (pp *pickleProcessor) readProcessEntry(i int, buf []byte) error {
	if pp.visited[i] != "" {
		return nil
	}

	offset := pp.ix[i]
	_, err := pp.r.Seek(int64(offset), io.SeekStart)
	if err != nil {
		return err
	}

	tag, err := readByte(pp.r, buf)
	if err != nil {
		return err
	}

	len, err := readLongNat(pp.r, buf)
	if err != nil {
		return err
	}

	return pp.processEntry(tag, len, i, buf)
}

func (pp *pickleProcessor) processEntry(tag byte, len int64, i int, buf []byte) error {
	end, err := pp.r.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	end += len

	switch tag {
	case 1, 2:
		_, err := io.ReadFull(pp.r, buf[:len])
		if err != nil {
			return err
		}
		name := string(buf[:len])
		pp.visited[i] = fmt.Sprintf("tag:%d (%s)", tag, name)
	case 4, 5, 7:
		err = pp.readProcessRef(i, "name_Ref", buf)
		if err != nil {
			return err
		}
		err = pp.readProcessRef(i, "owner_Ref", buf)
		if err != nil {
			return err
		}
		flag, err := readLongNat(pp.r, buf)
		if err != nil {
			return err
		}
		nextI, err := readLongNat(pp.r, buf)
		if err != nil {
			return err
		}
		pos, err := pp.r.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		privateWithinI := int64(-1)
		infoI := nextI
		if pos != end {
			privateWithinI = nextI
			infoI, err = readLongNat(pp.r, buf)
			if err != nil {
				return err
			}
		}

		pp.visited[i] = fmt.Sprintf("symbol:%d [%d]", tag, flag)

		if privateWithinI != -1 {
			err = pp.processRef(i, int(privateWithinI), "privateWithin_Ref", buf)
			if err != nil {
				return err
			}
		}

		err = pp.processRef(i, int(infoI), "info_Ref", buf)
		if err != nil {
			return err
		}
	default:
		fmt.Println("unsupported tag", tag)
		// 				Entry          = 1 TERMNAME len_Nat NameInfo
		//  *                  | 2 TYPENAME len_Nat NameInfo
		//  *                  | 3 NONEsym len_Nat
		//  *                  | 4 TYPEsym len_Nat SymbolInfo
		//  *                  | 5 ALIASsym len_Nat SymbolInfo
		//  *                  | 6 CLASSsym len_Nat SymbolInfo [thistype_Ref]
		//  *                  | 7 MODULEsym len_Nat SymbolInfo
		//  *                  | 8 VALsym len_Nat [defaultGetter_Ref /* no longer needed*/] SymbolInfo [alias_Ref]
		//  *                  | 9 EXTref len_Nat name_Ref [owner_Ref]
		//  *                  | 10 EXTMODCLASSref len_Nat name_Ref [owner_Ref]
		//  *                  | 11 NOtpe len_Nat
		//  *                  | 12 NOPREFIXtpe len_Nat
		//  *                  | 13 THIStpe len_Nat sym_Ref
		//  *                  | 14 SINGLEtpe len_Nat type_Ref sym_Ref
		//  *                  | 15 CONSTANTtpe len_Nat constant_Ref
		//  *                  | 16 TYPEREFtpe len_Nat type_Ref sym_Ref {targ_Ref}
		//  *                  | 17 TYPEBOUNDStpe len_Nat tpe_Ref tpe_Ref
		//  *                  | 18 REFINEDtpe len_Nat classsym_Ref {tpe_Ref}
		//  *                  | 19 CLASSINFOtpe len_Nat classsym_Ref {tpe_Ref}
		//  *                  | 20 METHODtpe len_Nat tpe_Ref {sym_Ref}
		//  *                  | 21 POLYTtpe len_Nat tpe_Ref {sym_Ref}
		//  *                  | 22 IMPLICITMETHODtpe len_Nat tpe_Ref {sym_Ref} /* no longer needed */
		//  *                  | 52 SUPERtpe len_Nat tpe_Ref tpe_Ref
		//  *                  | 24 LITERALunit len_Nat
		//  *                  | 25 LITERALboolean len_Nat value_Long
		//  *                  | 26 LITERALbyte len_Nat value_Long
		//  *                  | 27 LITERALshort len_Nat value_Long
		//  *                  | 28 LITERALchar len_Nat value_Long
		//  *                  | 29 LITERALint len_Nat value_Long
		//  *                  | 30 LITERALlong len_Nat value_Long
		//  *                  | 31 LITERALfloat len_Nat value_Long
		//  *                  | 32 LITERALdouble len_Nat value_Long
		//  *                  | 33 LITERALstring len_Nat name_Ref
		//  *                  | 34 LITERALnull len_Nat
		//  *                  | 35 LITERALclass len_Nat tpe_Ref
		//  *                  | 36 LITERALenum len_Nat sym_Ref
		//  *                  | 40 SYMANNOT len_Nat sym_Ref AnnotInfoBody
		//  *                  | 41 CHILDREN len_Nat sym_Ref {sym_Ref}
		//  *                  | 42 ANNOTATEDtpe len_Nat [sym_Ref /* no longer needed */] tpe_Ref {annotinfo_Ref}
		//  *                  | 43 ANNOTINFO len_Nat AnnotInfoBody
		//  *                  | 44 ANNOTARGARRAY len_Nat {constAnnotArg_Ref}
		//  *                  | 47 DEBRUIJNINDEXtpe len_Nat level_Nat index_Nat
		//  *                  | 48 EXISTENTIALtpe len_Nat type_Ref {symbol_Ref}
		//  *                  | 49 TREE len_Nat 1 EMPTYtree
		//  *                  | 49 TREE len_Nat 2 PACKAGEtree type_Ref sym_Ref mods_Ref name_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 3 CLASStree type_Ref sym_Ref mods_Ref name_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 4 MODULEtree type_Ref sym_Ref mods_Ref name_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 5 VALDEFtree type_Ref sym_Ref mods_Ref name_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 6 DEFDEFtree type_Ref sym_Ref mods_Ref name_Ref numtparams_Nat {tree_Ref} numparamss_Nat {numparams_Nat {tree_Ref}} tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 7 TYPEDEFtree type_Ref sym_Ref mods_Ref name_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 8 LABELtree type_Ref sym_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 9 IMPORTtree type_Ref sym_Ref tree_Ref {name_Ref name_Ref}
		//  *                  | 49 TREE len_Nat 11 DOCDEFtree type_Ref sym_Ref string_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 12 TEMPLATEtree type_Ref sym_Ref numparents_Nat {tree_Ref} tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 13 BLOCKtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 14 CASEtree type_Ref tree_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 15 SEQUENCEtree type_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 16 ALTERNATIVEtree type_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 17 STARtree type_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 18 BINDtree type_Ref sym_Ref name_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 19 UNAPPLYtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 20 ARRAYVALUEtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 21 FUNCTIONtree type_Ref sym_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 22 ASSIGNtree type_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 23 IFtree type_Ref tree_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 24 MATCHtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 25 RETURNtree type_Ref sym_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 26 TREtree type_Ref tree_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 27 THROWtree type_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 28 NEWtree type_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 29 TYPEDtree type_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 30 TYPEAPPLYtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 31 APPLYtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 32 APPLYDYNAMICtree type_Ref sym_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 33 SUPERtree type_Ref sym_Ref tree_Ref name_Ref
		//  *                  | 49 TREE len_Nat 34 THIStree type_Ref sym_Ref  name_Ref
		//  *                  | 49 TREE len_Nat 35 SELECTtree type_Ref sym_Ref tree_Ref name_Ref
		//  *                  | 49 TREE len_Nat 36 IDENTtree type_Ref sym_Ref name_Ref
		//  *                  | 49 TREE len_Nat 37 LITERALtree type_Ref constant_Ref
		//  *                  | 49 TREE len_Nat 38 TYPEtree type_Ref
		//  *                  | 49 TREE len_Nat 39 ANNOTATEDtree type_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 40 SINGLETONTYPEtree type_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 41 SELECTFROMTYPEtree type_Ref tree_Ref name_Ref
		//  *                  | 49 TREE len_Nat 42 COMPOUNDTYPEtree type_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 43 APPLIEDTYPEtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 49 TREE len_Nat 44 TYPEBOUNDStree type_Ref tree_Ref tree_Ref
		//  *                  | 49 TREE len_Nat 45 EXISTENTIALTYPEtree type_Ref tree_Ref {tree_Ref}
		//  *                  | 50 MODIFIERS len_Nat flags_Long privateWithin_Ref
	}
	_, err = pp.r.Seek(end+1, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

func readScalaSignature(r io.ReadSeeker) error {
	pp := &pickleProcessor{
		r: r,
	}
	buf := make([]byte, 512)
	var err error

	major, err := readLongNat(r, buf)
	if err != nil {
		return err
	}
	minor, err := readLongNat(r, buf)
	if err != nil {
		return err
	}
	fmt.Println("Version", major, minor)
	pp.ix, err = createIndex(r, buf)
	pp.visited = make([]string, len(pp.ix))
	fmt.Println("Table size: ", len(pp.ix))
	if err != nil {
		return err
	}
	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	for i := range pp.ix {
		pp.readProcessEntry(i, buf)
	}
	return nil
}
