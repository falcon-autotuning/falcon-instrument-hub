# Functions for use with the screening station qcodes implementation

from itertools import compress
from typing import Any

import nidaqmx
import numpy as np
import numpy.typing as npt
import pandas as pd
from nidaqmx import constants
from qcodes.dataset import Measurement
from qcodes.instrument_drivers.Keithley import Keithley2400

import falcon.drivers.sweep_datatypes as SDT
from falcon.datatypes import Channel, Gate, Ohmic

# from . import StateDF
from falcon.drivers.sweep_datatypes import Waveform1D
from .nidaq import NDictionary
from .qdac import (
    Leakage_Matrix_Collector,
    QDictionary,
)


class DataCollection:
    """Holds all of the data collection functions. Since the data collection is all high level, most functions require device parameters easily accesible from the dicts

    The Coldstart class instantiates this class to call these functions using all the collected metadata
    """

    _QDict: QDictionary
    _NDict: NDictionary
    _leaky: Leakage_Matrix_Collector
    _keith: Keithley2400
    _current_channels: list[Channel]
    _global_gates: list[Gate]
    _global_ohmics: list[Ohmic]
    _no_qdac_connection = []

    def __init__(
        self,
        QDict: QDictionary,  # Object containing QDAC information
        NDict: NDictionary,  # Object containing NiDAQ information
        keith: Keithley2400,  # Instrument driver for Keithley2400
        current_channels: list[Channel],  # List of channels containing current sensors
        global_gates: list[
            Gate
        ],  # List of gates that are global (i.e., all voltage sweeps are performed on these gates)
        global_ohmics: list[
            Ohmic
        ],  # List of ohmic sensors used for leakage measurements
        no_qdac_connection: list[
            str
        ],  # List of gates that don't have a QDAC connected to them
        leaky: Leakage_Matrix_Collector,  # Object collecting leakage data
    ):
        """Initialize dataCollection object with necessary parameters.

        Args:
            QDict (QDictionary): Object containing QDAC information.
            NDict (NDictionary): Object containing NiDAQ information.
            keith (Keithley2400): Instrument driver for Keithley2400.
            current_channels (list[Channel]): List of channels containing current sensors.
            global_gates (list[Gate]): List of gates that are global (i.e., all voltage sweeps are performed on these gates).
            global_ohmics (list[Ohmic]): List of ohmic sensors used for leakage measurements.
            no_qdac_connection (list[str]): List of gates that don't have a QDAC connected to them.
            leaky (Leakage_Matrix_Collector): Object collecting leakage data.
        """
        self._QDict = QDict  # Store QDAC information
        self._NDict = NDict  # Store NiDAQ information
        self._keith = keith  # Store Keithley2400 instrument driver
        self._current_channels = (
            current_channels  # Store channels containing current sensors
        )
        self._global_gates = global_gates  # Store list of global gates
        self._global_ohmics = global_ohmics  # Store list of ohmic sensors
        self._no_qdac_connection = (
            no_qdac_connection  # Store list of gates without QDAC connection
        )
        self._leaky = leaky  # Store leakage data collector

    # Configuration measurement utilities for binned sweeps
    def waveform1D(
        self,
        Channel1Inputs: Waveform1D,
        Channel1Sweep: dict[str, float],
    ):
        """High level goals: syncs QDAC's with NiDaq, calls programmingQDacFunctionGens to configure function generators,
        starts data collection, returns and saves data

        The supported wave forms are:
            sine = 1
            square = 2
            traingle = 3
            staircase = 4

        INPUTS
        pads : the names of the gates that are being set
        waveform : integer for the waveform to be programmed
        amplitude : amplitude of the waveform
        v_start : initial offset voltage of waveform
        repetitions : number of repitions of waveform
        sample_rate_per_channel : in Hz
        num_samples_raw :
        max_val : the max value of the NiDAQ input
        min_val : the min value of the NiDAQ input
        step_length : width of each step of the staircase, used only for staircase
        nsteps : number of steps, used only for staircase
        period: the period of the waveform, used only for triange, sine and square waves
        duty_cycle: percentage on or off, used only for square
        nidaq : instant of the NiDAQ6216 instrument

        We have a dictionary input Channel1Inputs such that each pad can have a custom amplitude, v_start, unit, and name. Other params are shared bw all waveforms and are generic inputs
        Channel1Inputs = {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        favorite will be a boolean parameter that can be used to highlight in database or smth

        Dictionary input Channel1Sweep for programming QDAC function generator
        {'waveform' : None, 'repetitions' : None, 'nsteps' : 10,'step_length' : 50, 'period' : 50, 'duty_cycle' : 50, 'slope' : 11}

        OUTPUT
        idnum : number corresponding to database entry
        """
        self._NDict.yaxis_labels()
        nidaqname = self._NDict._return_nidaq_name_from_pfi("trig1")
        nidaq = self._NDict._return_nidaq_object_from_name(nidaqname)

        # config
        self._QDict.selectMaster([value.value for value in Channel1Inputs.keys()])
        self._QDict.programmingQDacFunctionGens(
            Channel1Inputs=Channel1Inputs,
            Channel1SweepParams=Channel1Sweep,
        )
        self._QDict.QDACsync()
        num_samples_raw = int(
            Channel1Sweep["step_length"]
            * nidaq.n_bins.get()
            * nidaq.sample_rate.get()
            / 1000
        )
        qdac = self.find_master()
        trigger_channel = list(
            self._QDict._index_df[
                (self._QDict._index_df["QDac"] == qdac)
                & (self._QDict._index_df["Pad"].str.contains("trig"))
            ]["NIDaq Ch"]
        )[-1]
        assert isinstance(trigger_channel, str)
        pfichannel = self._NDict._return_pfi_channel_object_from_NIDaq_Ch(
            trigger_channel
        )
        pfiname = pfichannel.name[-4:]
        trig_nidaq_ch = "/" + nidaq.NiMAXname + "/" + pfiname

        # decides on a good timeout given the amount of measurements to take
        default_nidaq_timeout = 10
        timeout = int(1.1 * num_samples_raw / nidaq.sample_rate.get())
        if int(1.1 * num_samples_raw / nidaq.sample_rate.get()) < default_nidaq_timeout:
            timeout = default_nidaq_timeout
        # print("Starting keyboard interupt section")
        # collect -- nidaqmx.error_codes.DAQmxErrors() for debugging
        # with DelayedKeyboardInterrupt():
        with nidaqmx.Task("trigger_task") as trigger_task:
            trigger_task = nidaqmx.Task()
            for ch in nidaq.return_ai_channels():
                name = ch.name[-3:]
                trigger_task.ai_channels.add_ai_voltage_chan(
                    nidaq.NiMAXname + "/" + name,
                    max_val=ch.upVrail.get(),
                    min_val=ch.botVrail.get(),
                )
            trigger_task.triggers.start_trigger.cfg_dig_edge_start_trig(
                trig_nidaq_ch, trigger_edge=pfichannel.trigger_edge.get()
            )
            trigger_task.timing.cfg_samp_clk_timing(
                nidaq.sample_rate.get(),
                sample_mode=constants.AcquisitionType.FINITE,
                samps_per_chan=num_samples_raw,
            )
            trigger_task.start()
            # trigger
            self._QDict.trigger1D()
            raw_data = np.array(
                trigger_task.read(
                    number_of_samples_per_channel=num_samples_raw,
                    timeout=timeout,
                )
            )
            # trigger_task.wait_until_done()
            trigger_task.stop()
            trigger_task.close()
        i = 0
        data = {}
        for Iamp in nidaq.return_ai_channels():
            Iamp = Iamp.current_channel_name
            # print(Iamp, type(Iamp), "is the key")
            data[Iamp] = raw_data[i]
            i += 1
        data["time"] = np.linspace(
            0, num_samples_raw / nidaq.sample_rate.get(), num_samples_raw
        )
        return data

    def find_master(self) -> str:
        """Finds the master QDAC"""
        for qdac in list(self._QDict._qdacs.keys()):
            if self._QDict._qdacconfig[qdac] == "master":
                return qdac
        raise ValueError("Found no master")

    def waveform2D(
        self,
        Channel1Inputs,
        Channel2Inputs,
        Channel1SweepParams,
        Channel2SweepParams,
        verbose=False,
    ):
        """High level goals: syncs QDAC's with NiDaq, calls programmingQDacFunctionGens to configure function generators,
        starts data collection, returns and saves data

        The supported wave forms are:
            sine = 1
            square = 2
            traingle = 3
            staircase = 4

        INPUTS
        Channel1Inputs dictionary for the programming of the 1st axis (fast)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        Channel2Inputs dictionary for the programming of the 2nd axis (slow)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}

        Channel1SweepParams dictionary for the programming of the waveform settings of the 1st channel
        {'waveform' : None, 'repetitions' : None, 'nsteps' : 10,'step_length' : 50, 'period' : 50, 'duty_cycle' : 50, 'slope' : 11}
        Channel2SweepParams dictionary for the programming of the waveform settings of the 1st channel
        {'waveform' : None, 'repetitions' : None, 'nsteps' : 10,'step_length' : 50, 'period' : 50, 'duty_cycle' : 50, 'slope' : 11}

        OUTPUT
        idnum : number corresponding to database entry
        """
        self._NDict.yaxis_labels()
        # self._NDict.xaxis_labels(pads, name, unit) # Deprecated needs to be updated
        nidaqname = self._NDict._return_nidaq_name_from_pfi("trig1")
        nidaq = self._NDict._return_nidaq_object_from_name(nidaqname)

        # config
        self._QDict.selectMaster(
            list(Channel2Inputs.keys())
        )  # Should select slow qdac first

        Channel1SweepParams["nsteps"] = nidaq.nx_bins.get()
        Channel2SweepParams["nsteps"] = nidaq.ny_bins.get()
        self._QDict.programmingQDacFunctionGens(
            Channel1Inputs, Channel1SweepParams=Channel1SweepParams
        )
        self._QDict.programmingQDacFunctionGens(
            Channel2Inputs, Channel1SweepParams=Channel2SweepParams
        )
        self._QDict.TurboQDACsync(slowpads=list(Channel1Inputs.keys()))
        num_samples_raw = nidaq.n_points.get()
        qdac = self.find_master()
        trigger_channel = list(
            self._QDict._index_df[
                (self._QDict._index_df["QDac"] == qdac)
                & (self._QDict._index_df["Pad"].str.contains("trig"))
            ]["NIDaq Ch"]
        )[-1]

        pfichannel = self._NDict._return_pfi_channel_object_from_NIDaq_Ch(
            trigger_channel
        )
        pfiname = pfichannel.name[-4:]
        trig_nidaq_ch = "/" + nidaq.NiMAXname + "/" + pfiname

        # decides on a good timeout given the amount of measurements to take
        default_nidaq_timeout = 10
        timeout = int(1.1 * num_samples_raw / nidaq.sample_rate.get())
        if int(1.1 * num_samples_raw / nidaq.sample_rate.get()) < default_nidaq_timeout:
            timeout = default_nidaq_timeout
        if verbose:
            print(f"printing trigger config {self._QDict._triggerconfig}")
        # collect -- nidaqmx.error_codes.DAQmxErrors() for debugging
        # with DelayedKeyboardInterrupt():
        with nidaqmx.Task("trigger_task") as trigger_task:
            trigger_task = nidaqmx.Task()
            for ch in nidaq.return_ai_channels():
                name = ch.name[-3:]
                trigger_task.ai_channels.add_ai_voltage_chan(
                    nidaq.NiMAXname + "/" + name,
                    max_val=ch.upVrail.get(),
                    min_val=ch.botVrail.get(),
                )
            trigger_task.triggers.start_trigger.cfg_dig_edge_start_trig(
                trig_nidaq_ch, trigger_edge=pfichannel.trigger_edge.get()
            )
            trigger_task.timing.cfg_samp_clk_timing(
                nidaq.sample_rate.get(),
                sample_mode=constants.AcquisitionType.FINITE,
                samps_per_chan=num_samples_raw,
            )
            trigger_task.start()
            # trigger
            self._QDict.trigger1D()
            raw_data = np.array(
                trigger_task.read(
                    number_of_samples_per_channel=num_samples_raw, timeout=timeout
                )
            )
            # trigger_task.wait_until_done()
            trigger_task.stop()
            trigger_task.close()
        i = 0
        data = {}
        for Iamp in nidaq.return_ai_channels():
            Iamp = Iamp.current_channel_name
            data[Iamp] = raw_data[i]
            i += 1
        data["time"] = np.linspace(
            start=0,
            stop=(num_samples_raw - 1) / nidaq.sample_rate.get(),
            num=num_samples_raw,
        )
        return data

    # Default measurement functions
    def plot1D(
        self,
        Channel1Inputs: Waveform1D,
        nsteps: int,
    ):
        """Does a non buffered 1D plot similar to Labber.

        Must set nidaq.n_points before taking measurement!

        INPUTS
        Channel1Inputs dictionary for the programming of the 1st axis (fast)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        nsteps : the number of steps in the sweep

        OUTPUT
        idnum : number corresponding to database entry
        """
        favorite_pad = Channel1Inputs.get_favorite()

        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        num_samples_raw = nidaq.n_points.get()
        nidaq.n_bins(1)
        self._NDict.xaxis_labels(
            channel1pads=favorite_pad, name1=Channel1Inputs[favorite_pad].name
        )
        default_nidaq_timeout = 10
        timeout = int(1.1 * num_samples_raw / nidaq.sample_rate.get())
        if int(1.1 * num_samples_raw / nidaq.sample_rate.get()) < default_nidaq_timeout:
            timeout = default_nidaq_timeout

        meas = Measurement()
        self._NDict.yaxis_labels()
        p_dict = {}  # preallocated_dict
        channelVoltage_array = {}
        meas.register_parameter(
            self._QDict._return_channel_object(favorite_pad.value).v
        )  # register independent variable first
        for pad in list(
            Channel1Inputs.keys()
        ):  # Generating all dependent voltage sweeps
            channelVoltage_array[pad] = np.linspace(
                Channel1Inputs[pad].v_start,
                Channel1Inputs[pad].v_start + Channel1Inputs[pad].amplitude,
                nsteps,
            )
        for current_to_be_stored in self._current_channels:
            # Preallocating measurement storage parameters
            trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).voltage_traces
            aver_value = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).aver_value
            gain = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).gain.get()
            offset = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).offset.get()
            p_dict[current_to_be_stored] = {
                "trace": trace,
                "aver_value": aver_value,
                "gain": gain,
                "offset": offset,
            }
            meas.register_parameter(trace)
            meas.register_parameter(
                aver_value,
                setpoints=(self._QDict._return_channel_object(favorite_pad.value).v,),
            )  # register dependent variable

        # Voltage gets set to v_start before the first measurement is taken

        for pad in list(Channel1Inputs.keys()):
            self._QDict.qdacVset(pad.value, channelVoltage_array[pad][0])

        try:
            with meas.run() as datasaver:
                dataid = datasaver.run_id
                for i in range(0, nsteps):
                    for pad in list(Channel1Inputs.keys()):
                        self._QDict._return_qdac_object_from_channel(pad.value).write(
                            f"set {self._QDict._return_channel_number(pad.value)} {channelVoltage_array[pad][i]:.6f}"
                        )
                        # self._QDict.qdacVset(pad, channelVoltage_array[pad][i], slope = 1000)#_return_channel_object(pad).v.set(channelVoltage_array[pad][i], slope=False)
                        # qdacVset(pad, channelVoltage_array[pad][i])
                    # with DelayedKeyboardInterrupt():
                    with nidaqmx.Task("ai_task") as ai_task:
                        for ch in nidaq.return_ai_channels():
                            name = ch.name[-3:]
                            ai_task.ai_channels.add_ai_voltage_chan(
                                nidaq.NiMAXname + "/" + name,
                                max_val=ch.upVrail.get(),
                                min_val=ch.botVrail.get(),
                            )
                        ai_task.timing.cfg_samp_clk_timing(
                            nidaq.sample_rate.get(),
                            sample_mode=constants.AcquisitionType.FINITE,
                            samps_per_chan=num_samples_raw,
                        )
                        ai_task.start()
                        raw_data = np.array(
                            ai_task.read(
                                number_of_samples_per_channel=num_samples_raw,
                                timeout=timeout,
                            )
                        )
                        ai_task.wait_until_done()
                        ai_task.stop()

                        data = {}
                        data["time"] = np.linspace(
                            0,
                            num_samples_raw / nidaq.sample_rate.get(),
                            num_samples_raw,
                        )
                        value = {}
                        j = 0
                        for Iamp in nidaq.return_ai_channels():
                            Iamp = Iamp.current_channel_name
                            data[Iamp] = raw_data[j]
                            value[Iamp] = np.mean(data[Iamp])
                            j += 1
                        for current_to_be_stored in self._current_channels:
                            # print(current_to_be_stored)
                            datasaver.add_result(
                                (nidaq.time_axis, data["time"]),
                                (
                                    p_dict[current_to_be_stored]["trace"],
                                    p_dict[current_to_be_stored]["gain"]
                                    * (
                                        data[current_to_be_stored]
                                        + p_dict[current_to_be_stored]["offset"]
                                    ),
                                ),
                            )
                            datasaver.add_result(
                                (
                                    self._QDict._return_channel_object(
                                        favorite_pad.value
                                    ).v,
                                    channelVoltage_array[favorite_pad][i],
                                ),
                                (
                                    p_dict[current_to_be_stored]["aver_value"],
                                    p_dict[current_to_be_stored]["gain"]
                                    * (
                                        value[current_to_be_stored]
                                        + p_dict[current_to_be_stored]["offset"]
                                    ),
                                ),
                            )

                # appending metadata
                state_df = self.create_state_df(channel1Inputs=Channel1Inputs)
                dataframeDict = {
                    "index_df": self._NDict._index_df,
                    "state_df": state_df,
                }
                self.dataframeToMetadata(
                    dataframeDict=dataframeDict, datasaver=datasaver
                )
                return dataid
        except KeyboardInterrupt:
            # Try and save whatever data was taken
            # appending metadata
            state_df = self.create_state_df(channel1Inputs=Channel1Inputs)
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)
        return dataid

    def plot2D(
        self,
        Channel1Inputs: Waveform1D,
        Channel2Inputs: Waveform1D,
        n1steps: int,
        n2steps: int,
        slope: int = 50,
    ):
        """Does a non buffered 2D plot similar to Labber.

        Must set nidaq.n_points before taking measurement!

        INPUTS
        Channel1Inputs dictionary for the programming of the 1st axis (fast)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        Channel2Inputs dictionary for the programming of the 1st axis (fast)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        n1steps : the number of steps in the Channel1 direction of the sweep
        n2steps : the number of steps in the Channel2 direction of the sweep
        slope : in V/sec we find 11V/sec is not fast enough to take 1V to 0V without errors

        OUTPUT
        idnum : number corresponding to database entry
        """
        favorite_pad1 = Channel1Inputs.get_favorite()
        favorite_pad2 = Channel2Inputs.get_favorite()

        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        num_samples_raw = nidaq.n_points.get()
        nidaq.n_bins(1)
        self._NDict.xaxis_labels(
            channel1pads=favorite_pad1,
            name1=Channel1Inputs[favorite_pad1].name,
            channel2pads=favorite_pad2,
            name2=Channel2Inputs[favorite_pad2].name,
        )
        default_nidaq_timeout = 10
        timeout = int(1.1 * num_samples_raw / nidaq.sample_rate.get())
        if int(1.1 * num_samples_raw / nidaq.sample_rate.get()) < default_nidaq_timeout:
            timeout = default_nidaq_timeout

        meas = Measurement()
        self._NDict.yaxis_labels()
        p_dict = {}  # preallocated_dict
        channel1Voltage_array = {}
        channel2Voltage_array = {}
        meas.register_parameter(
            self._QDict._return_channel_object(favorite_pad1.value).v
        )  # register independent variable first
        meas.register_parameter(
            self._QDict._return_channel_object(favorite_pad2.value).v
        )  # register independent variable first

        for pad in list(
            Channel1Inputs.keys()
        ):  # Generating all dependent voltage sweeps
            channel1Voltage_array[pad] = np.linspace(
                Channel1Inputs[pad].v_start,
                Channel1Inputs[pad].v_start + Channel1Inputs[pad].amplitude,
                n1steps,
            )

        for pad in list(
            Channel2Inputs.keys()
        ):  # Generating all dependent voltage sweeps
            channel2Voltage_array[pad] = np.linspace(
                Channel2Inputs[pad].v_start,
                Channel2Inputs[pad].v_start + Channel2Inputs[pad].amplitude,
                n2steps,
            )

        # print(channel2Voltage_array)

        for current_to_be_stored in self._current_channels:
            # Preallocating measurement storage parameters
            trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).voltage_traces
            aver_value = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).aver_value
            gain = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).gain.get()
            offset = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).offset.get()
            p_dict[current_to_be_stored] = {
                "trace": trace,
                "aver_value": aver_value,
                "gain": gain,
                "offset": offset,
            }
            meas.register_parameter(trace)
            meas.register_parameter(
                aver_value,
                setpoints=(
                    self._QDict._return_channel_object(favorite_pad1.value).v,
                    self._QDict._return_channel_object(favorite_pad2.value).v,
                ),
            )  # register dependent variable

        # Voltage gets set to v_start before the first measurement is taken

        for pad in list(Channel2Inputs.keys()):
            self._QDict.qdacVset(pad.value, channel2Voltage_array[pad][0])

        for pad in list(Channel1Inputs.keys()):
            self._QDict.qdacVset(pad.value, channel1Voltage_array[pad][0])

        try:
            with meas.run() as datasaver:
                dataid = datasaver.run_id
                for iter1 in range(0, n1steps):
                    for pad in list(Channel1Inputs.keys()):
                        self._QDict.qdacVset(
                            pad.value,
                            channel1Voltage_array[pad][iter1],
                            slope=slope,
                        )
                        # print(f"{pad} is set to {channel1Voltage_array[pad][iter1]}")
                    for iter2 in range(0, n2steps):
                        for pad in list(Channel2Inputs.keys()):
                            self._QDict.qdacVset(
                                pad.value,
                                channel2Voltage_array[pad][iter2],
                                slope=slope,
                            )
                            # print(f"{pad} is set to {channel2Voltage_array[pad][iter2]}")
                        # time.sleep(5/11)
                        # print(f"slept for {5/11}")
                        # with DelayedKeyboardInterrupt():
                        with nidaqmx.Task("ai_task") as ai_task:
                            for ch in nidaq.return_ai_channels():
                                name = ch.name[-3:]
                                ai_task.ai_channels.add_ai_voltage_chan(
                                    nidaq.NiMAXname + "/" + name,
                                    max_val=ch.upVrail.get(),
                                    min_val=ch.botVrail.get(),
                                )
                            ai_task.timing.cfg_samp_clk_timing(
                                nidaq.sample_rate.get(),
                                sample_mode=constants.AcquisitionType.FINITE,
                                samps_per_chan=num_samples_raw,
                            )
                            ai_task.start()
                            raw_data = np.array(
                                ai_task.read(
                                    number_of_samples_per_channel=num_samples_raw,
                                    timeout=timeout,
                                )
                            )
                            ai_task.wait_until_done()
                            ai_task.stop()
                            data = {}
                            data["time"] = np.linspace(
                                0,
                                num_samples_raw / nidaq.sample_rate.get(),
                                num_samples_raw,
                            )
                            value = {}
                            k = 0
                            # print("taking data")
                            for Iamp in nidaq.return_ai_channels():
                                Iamp = Iamp.current_channel_name
                                data[Iamp] = raw_data[k]
                                value[Iamp] = np.mean(data[Iamp])
                                k += 1
                            for current_to_be_stored in self._current_channels:
                                # np.set_printoptions(suppress = False)
                                # print(f"{current_to_be_stored} {p_dict[current_to_be_stored]['gain']*(data[current_to_be_stored]+p_dict[current_to_be_stored]['offset'])}")
                                datasaver.add_result(
                                    (nidaq.time_axis, data["time"]),
                                    (
                                        p_dict[current_to_be_stored]["trace"],
                                        p_dict[current_to_be_stored]["gain"]
                                        * (
                                            data[current_to_be_stored]
                                            + p_dict[current_to_be_stored]["offset"]
                                        ),
                                    ),
                                )
                                datasaver.add_result(
                                    (
                                        self._QDict._return_channel_object(
                                            favorite_pad1.value
                                        ).v,
                                        channel1Voltage_array[favorite_pad1][iter1],
                                    ),
                                    (
                                        self._QDict._return_channel_object(
                                            favorite_pad2.value
                                        ).v,
                                        channel2Voltage_array[favorite_pad2][iter2],
                                    ),
                                    (
                                        p_dict[current_to_be_stored]["aver_value"],
                                        p_dict[current_to_be_stored]["gain"]
                                        * (
                                            value[current_to_be_stored]
                                            + p_dict[current_to_be_stored]["offset"]
                                        ),
                                    ),
                                )

                # appending metadata
                state_df = self.create_state_df(
                    channel1Inputs=Channel1Inputs, channel2Inputs=Channel2Inputs
                )
                dataframeDict = {
                    "index_df": self._NDict._index_df,
                    "state_df": state_df,
                }
                self.dataframeToMetadata(
                    dataframeDict=dataframeDict, datasaver=datasaver
                )
        except KeyboardInterrupt:
            # Try and save whatever data was taken
            # appending metadata
            state_df = self.create_state_df(
                channel1Inputs=Channel1Inputs, channel2Inputs=Channel2Inputs
            )
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)
        return dataid

    def sweep1D(
        self,
        Channel1Inputs: Waveform1D,
        nsteps: int,
        step_length: int = 50,
        risetime=0,
    ):
        """Abstracts waveform1D to only staircase waveforms with default values for typical measurements.

        Performs averaging to data

        We have a dictionary input Channel1Inputs such that each pad can have a custom amplitude, v_start, unit, and name. Other params are shared bw all waveforms and are generic inputs
        Channel1Inputs = {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        favorite will be select which x axis will be present in the database, all others will be referenced to it. Default value is False when read in.

        INPUTS
        pads : the names of the gates that are being set
        amplitude : amplitude of the waveform
        v_start : initial offset voltage of waveform
        step_length : width of each step of the staircase
        risetime : estimate of the rise time of the RC timeconstant of the fridge for more accurate averaging
        nsteps : number of steps, used only for staircase

        OUTPUT
        idnum : number corresponding to database entry
        """
        # nsteps must be an integer
        nsteps = int(nsteps)

        pads, v_starts, amplitudes, favorites, favorite_pad = (
            self.unpack_sweep_dict_params(Channel1Inputs)
        )

        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        nidaq.n_bins(
            nsteps
        )  # updates nidaq with regard to users inputs before programming it
        nidaq.n_points(
            int(step_length * nsteps * nidaq.sample_rate.get() / 1000)
        )  # the step length is in msec

        # Ramping all pads off to v_start
        for gate in list(Channel1Inputs.keys()):
            self._QDict.qdacVset(gate.value, Channel1Inputs[gate].v_start)

        value: dict[str, Any] = {}
        wave = SDT.waveformMaker()
        meas = Measurement()
        self._NDict.yaxis_labels()
        name = Channel1Inputs[favorite_pad].name
        if not isinstance(name, str):
            name = name.value
        self._NDict.xaxis_labels(
            channel1pads=favorite_pad.value,
            name1=name,
        )
        for current_to_be_stored in self._current_channels:
            # print("for loop iteration")
            # print(current_to_be_stored, type(current_to_be_stored))
            trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).voltage_traces
            aver_trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).aver_voltage
            meas.register_parameter(trace)
            meas.register_parameter(aver_trace)
        # print("Beginning measurement")
        with meas.run() as datasaver:
            # Doing the measurement and collecting the data
            # print("Beginning waveform1D")
            data = self.waveform1D(
                Channel1Inputs=Channel1Inputs,
                Channel1Sweep=wave.empty_sweep_builder_for_waveform1D(
                    waveform=4,
                    repetitions=1,
                    nsteps=nsteps,
                    step_length=step_length,
                    slope=11,
                ),
            )
            # print("Finished waveform1D")
            value = self._NDict.trace_averaging(data, step_length, risetime)
            value["voltage"] = np.linspace(
                start=list(compress(v_starts, favorites))[0],
                stop=list(compress(v_starts, favorites))[0]
                + list(compress(amplitudes, favorites))[0],
                num=nsteps,
            )

            # Logging data into database in proper format
            # print("Beginning to append data to the database")
            for current_to_be_stored in self._current_channels:
                trace = self._NDict._return_ai_channel_object(
                    current_to_be_stored.name
                ).voltage_traces
                aver_trace = self._NDict._return_ai_channel_object(
                    current_to_be_stored.name
                ).aver_voltage
                gain = self._NDict._return_ai_channel_object(
                    current_to_be_stored.name
                ).gain.get()
                offset = self._NDict._return_ai_channel_object(
                    current_to_be_stored.name
                ).offset.get()
                datasaver.add_result(
                    (nidaq.time_axis, value["time"]),
                    (trace, gain * (value[current_to_be_stored.name][0] + offset)),
                )
                datasaver.add_result(
                    (nidaq.bin_axis, value["voltage"]),
                    (aver_trace, gain * (value[current_to_be_stored.name][1] + offset)),
                )
            dataid = datasaver.run_id

            # appending metadata
            # print("No df made yet")
            state_df = self.create_state_df(channel1Inputs=Channel1Inputs)
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            # print(dataframeDict["index_df"])
            # print(dataframeDict["state_df"])
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)

        # resetting qdac for next measurements
        self._QDict.revertPadsToDC()
        self._QDict.QDACunsync()
        self._QDict.uncoupleQdacs([value.value for value in Channel1Inputs.keys()])
        self._QDict.disableQDacFunctionGens(4)

        return dataid

    def sweep2D(
        self,
        Channel1Inputs: Waveform1D,
        Channel2Inputs: Waveform1D,
        n1steps: int = 51,
        n2steps: int = 51,
        step_length: int = 50,
        risetime: int = 0,
    ):
        """Abstracts waveform1D to only staircase waveforms with default values for typical measurements.

        Performs averaging to data

        We have a dictionary input Channel1Inputs such that each pad can have a custom amplitude, v_start, unit, and name. Other params are shared bw all waveforms and are generic inputs
        Channel1Inputs = {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        favorite will be select which x axis will be present in the database, all others will be referenced to it. Default value is False when read in.

        INPUTS
        step_length : width of each step of the staircase
        risetime : estimate of the rise time of the RC timeconstant of the fridge for more accurate averaging
        n1steps : number of steps used with Channel1Inputs, used only for staircase
        n2steps : number of steps used with Channel2Inputs, used only for staircase

        OUTPUT
        idnum : number corresponding to database entry
        """
        # Getting data out of channel 1 input dict
        pads1, v_starts1, amplitudes1, favorites1, favorite_pad1 = (
            self.unpack_sweep_dict_params(Channel1Inputs)
        )

        # Getting data out of channel 2 input dict
        pads2, v_starts2, amplitudes2, favorites2, favorite_pad2 = (
            self.unpack_sweep_dict_params(Channel2Inputs)
        )

        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        nidaq.n_bins(
            n1steps
        )  # updates nidaq with regard to users inputs before programming it
        nidaq.n_points(
            int(step_length * nidaq.n_bins.get() * nidaq.sample_rate.get() / 1000)
        )  # the step length is in msec
        channel2Voltage_array: dict[str, npt.NDArray[np.floating[Any]]] = {}
        for pad in list(Channel2Inputs.keys()):
            channel2Voltage_array[pad.value] = np.linspace(
                Channel2Inputs[pad].v_start,
                Channel2Inputs[pad].v_start + Channel2Inputs[pad].amplitude,
                n2steps,
            )

        value = {}
        meas = Measurement()
        self._NDict.yaxis_labels()
        name1 = Channel1Inputs[favorite_pad1].name
        if not isinstance(name1, str):
            name1 = name1.value
        name2 = Channel2Inputs[favorite_pad2].name
        if not isinstance(name2, str):
            name2 = name2.value
        self._NDict.xaxis_labels(
            channel1pads=favorite_pad1.value,
            channel2pads=favorite_pad2.value,
            name1=name1,
            name2=name2,
        )
        meas.register_parameter(
            self._QDict._return_channel_object(favorite_pad2.value).v
        )  # Registering an independent parameter first
        wave = SDT.waveformMaker()
        # 2D slow axis label
        if Channel2Inputs[favorite_pad2].name is not None:
            old_name = self._QDict._return_channel_object(favorite_pad2.value).v.label
            self._QDict._return_channel_object(
                favorite_pad2.value
            ).v.label = Channel2Inputs[favorite_pad2].name

        p_dict: dict[str, dict[str, Any]] = {}  # preallocated_dict
        for current_to_be_stored in self._current_channels:
            # Preallocating measurement storage parameters
            trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).voltage_traces
            aver_trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).aver_voltage
            gain = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).gain.get()
            offset = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).offset.get()
            p_dict[current_to_be_stored.name] = {
                "trace": trace,
                "aver_trace": aver_trace,
                "gain": gain,
                "offset": offset,
            }
            meas.register_parameter(
                trace,
                setpoints=(self._QDict._return_channel_object(favorite_pad2.value).v,),
            )
            meas.register_parameter(
                aver_trace,
                setpoints=(self._QDict._return_channel_object(favorite_pad2.value).v,),
            )

        with meas.run() as datasaver:
            try:
                # Doing the measurement and collecting the data
                for i in range(0, n2steps):  # Outside for loop
                    for pad in list(
                        Channel2Inputs.keys()
                    ):  # Setting all of the pads seperately since they all have v_starts and amplitudes that are potentially different
                        self._QDict._return_qdac_object_from_channel(pad.value).write(
                            f"set {self._QDict._return_channel_number(pad.value)} {channel2Voltage_array[pad.value][i]:.6f}"
                        )
                        # self._QDict.qdacVset(pad, channel2Voltage_array[pad][i]) # inefficient all channels per qdac could be set simultaneously
                    data = self.waveform1D(
                        Channel1Inputs=Channel1Inputs,
                        Channel1Sweep=wave.empty_sweep_builder_for_waveform1D(
                            waveform=4,
                            repetitions=1,
                            nsteps=n1steps,
                            step_length=step_length,
                        ),
                    )
                    value = self._NDict.trace_averaging(data, step_length, risetime)
                    value["voltage"] = np.linspace(
                        start=list(compress(v_starts1, favorites1))[0],
                        stop=list(compress(v_starts1, favorites1))[0]
                        + list(compress(amplitudes1, favorites1))[0],
                        num=n1steps,
                    )
                    # For performance we know what voltage was actually set so we will substitute it below
                    getV = channel2Voltage_array[favorite_pad2.value][i]

                    # Logging data into database in proper format
                    for current_to_be_stored in self._current_channels:
                        datasaver.add_result(
                            (nidaq.time_axis, value["time"]),
                            (
                                p_dict[current_to_be_stored.name]["trace"],
                                p_dict[current_to_be_stored.name]["gain"]
                                * (
                                    value[current_to_be_stored.name][0]
                                    + p_dict[current_to_be_stored.name]["offset"]
                                ),
                            ),
                            (
                                self._QDict._return_channel_object(
                                    favorite_pad2.value
                                ).v,
                                getV,
                            ),
                        )
                        datasaver.add_result(
                            (nidaq.bin_axis, value["voltage"]),
                            (
                                p_dict[current_to_be_stored.name]["aver_trace"],
                                p_dict[current_to_be_stored.name]["gain"]
                                * (
                                    value[current_to_be_stored.name][1]
                                    + p_dict[current_to_be_stored.name]["offset"]
                                ),
                            ),
                            (
                                self._QDict._return_channel_object(
                                    favorite_pad2.value
                                ).v,
                                getV,
                            ),
                        )
            except KeyboardInterrupt:  # skips the rest of the measurement
                pass
            # Calling create_state_df to store all gate voltage/sweep metadata
            state_df = self.create_state_df(
                channel1Inputs=Channel1Inputs,
                channel2Inputs=Channel2Inputs,
            )

            # appending metadata
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)
            dataid = datasaver.run_id

        # resetting qdac for next measurements
        self._QDict.revertPadsToDC()
        self._QDict.QDACunsync()
        self._QDict.uncoupleQdacs([value.value for value in Channel1Inputs.keys()])
        self._QDict.disableQDacFunctionGens(4)
        # disabling coupling. The primary QDAC is selected by channel 1 inputs only in a plot1D fashion

        # Resetting QDAC labelling without custom name
        if Channel2Inputs[favorite_pad2].name is not None:
            self._QDict._return_channel_object(favorite_pad2.value).v.label = old_name

        return dataid

    def sweep2DTurbo(
        self,
        Channel1Inputs: Waveform1D,
        Channel2Inputs: Waveform1D,
        Channel1SweepParams,
        Channel2SweepParams,
        risetime: int = 0,
    ):
        """Fast version of a 2d sweep that utilizes the awg to play the entire sweep as one waveform.

        Performs averaging to data

        INPUTS

        Channel1Inputs dictionary for the programming of the 1st axis (fast)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}
        Channel2Inputs dictionary for the programming of the 2nd axis (slow)
        {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}

        Channel1SweepParams dictionary for the programming of the waveform settings of the 1st channel
        {'waveform' : None, 'repetitions' : None, 'nsteps' : 10,'step_length' : 50, 'period' : 50, 'duty_cycle' : 50, 'slope' : 11}
        Channel2SweepParams dictionary for the programming of the waveform settings of the 1st channel
        {'waveform' : None, 'repetitions' : None, 'nsteps' : 10,'step_length' : 50, 'period' : 50, 'duty_cycle' : 50, 'slope' : 11}
        risetime : estimate of the rise time of the RC timeconstant of the fridge for more accurate averaging

        OUTPUT
        data : dictionary of currents, first is channel name, then raw data, then averaged data
        """
        # Unpacking dictionaries
        pads1, v_starts1, amplitudes1, favorites1, favorite_pad1 = (
            self.unpack_sweep_dict_params(Channel1Inputs)
        )
        pads2, v_starts2, amplitudes2, favorites2, favorite_pad2 = (
            self.unpack_sweep_dict_params(Channel2Inputs)
        )

        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        try:
            (voltage, t) = self._QDict.turboVoltage(
                amplitude=Channel1Inputs[favorite_pad1].amplitude,
                step_width=Channel1SweepParams["step_length"],
                num_steps=Channel1SweepParams["nsteps"],
                slope=Channel1SweepParams["slope"],
            )
        except IndexError:
            (voltage, t) = self._QDict.turboVoltage(
                amplitude=Channel1Inputs[favorite_pad1].amplitude,
                step_width=Channel1SweepParams["step_length"],
                num_steps=Channel1SweepParams["nsteps"],
            )  # default value of 11 slope if not specified
        Channel2SweepParams["step_length"] = len(voltage)
        # if verbose : print(f"The step_width of each slow step should be {Channel2SweepParams['step_length']}")
        # if verbose : print(voltage)

        nidaq.nx_bins(
            Channel1SweepParams["nsteps"]
        )  # updates nidaq with regard to users inputs before programming it
        nidaq.ny_bins(
            Channel2SweepParams["nsteps"]
        )  # updates nidaq with regard to users inputs before programming it
        nidaq.n_points(
            int(
                Channel2SweepParams["step_length"]
                * nidaq.ny_bins.get()
                * nidaq.sample_rate.get()
                / 1000
            )
        )  # the step length is in msec
        # if verbose : print(f" n_points should be {int(Channel2SweepParams['step_length']*nidaq.ny_bins.get()*nidaq.sample_rate.get()/1000)}")
        # if verbose : print(nidaq.n_points.get())

        value = {}
        meas = Measurement()
        self._NDict.yaxis_labels()
        self._NDict.xaxis_labels(
            channel1pads=favorite_pad1,
            channel2pads=favorite_pad2,
            name1=Channel1Inputs[favorite_pad1].name,
            name2=Channel2Inputs[favorite_pad2].name,
        )

        # 2D slow axis label
        try:
            if Channel2Inputs[favorite_pad2].name is not None:
                old_name = self._QDict._return_channel_object(
                    favorite_pad2.value
                ).v.label
                self._QDict._return_channel_object(
                    favorite_pad2.value
                ).v.label = Channel2Inputs[favorite_pad2].name
        except IndexError:
            pass

        p_dict = {}  # preallocated_dict
        for current_to_be_stored in self._current_channels:
            # Preallocating measurement storage parameters
            turbo_array = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).turbo_array
            aver_turbo_array = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).aver_turbo_array
            gain = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).gain.get()
            offset = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).offset.get()
            p_dict[current_to_be_stored] = {
                "trace": turbo_array,
                "aver_turbo_array": aver_turbo_array,
                "gain": gain,
                "offset": offset,
            }
            meas.register_parameter(turbo_array)
            meas.register_parameter(aver_turbo_array)

        with meas.run() as datasaver:
            # Doing the measurement and collecting the data
            # if verbose : print(f"Before waveform2d {nidaq.n_points.get()}")
            data = self.waveform2D(
                Channel1Inputs=Channel1Inputs,
                Channel2Inputs=Channel2Inputs,
                Channel1SweepParams=Channel1SweepParams,
                Channel2SweepParams=Channel2SweepParams,
            )
            # if verbose : print(f"After waveform2d {nidaq.n_points.get()}")
            value = self._NDict.trace_turbo_averaging(
                data,
                Channel1SweepParams["step_length"],
                risetime,
                turboVoltagetimeLength=len(voltage),
            )
            x = np.linspace(
                Channel1Inputs[favorite_pad1].v_start,
                Channel1Inputs[favorite_pad1].v_start
                + Channel1Inputs[favorite_pad1].amplitude,
                Channel1SweepParams["nsteps"],
            )
            y = np.linspace(
                Channel2Inputs[favorite_pad2].v_start,
                Channel2Inputs[favorite_pad2].v_start
                + Channel2Inputs[favorite_pad2].amplitude,
                Channel2SweepParams["nsteps"],
            )
            # full coordinate arrays
            xx, yy = np.meshgrid(x, y, indexing="ij")
            # if verbose : print(f' xx shape is {xx.shape}')
            # if verbose : print(f' yy shape is {yy.shape}')

            # Logging data into database in proper format
            for current_to_be_stored in self._current_channels:
                datasaver.add_result(
                    (nidaq.turbo_time, value["time"]),
                    (
                        p_dict[current_to_be_stored]["trace"],
                        p_dict[current_to_be_stored]["gain"]
                        * (
                            value[current_to_be_stored][0]
                            + p_dict[current_to_be_stored]["offset"]
                        ),
                    ),
                )
                datasaver.add_result(
                    (nidaq.turbo_x_bin_array, xx),
                    (nidaq.turbo_y_bin_array, yy),
                    (
                        p_dict[current_to_be_stored]["aver_turbo_array"],
                        (
                            p_dict[current_to_be_stored]["gain"]
                            * (
                                value[current_to_be_stored][1]
                                + p_dict[current_to_be_stored]["offset"]
                            )
                        ).T,
                    ),
                )
            # Calling create_state_df to store all gate voltage/sweep metadata
            state_df = self.create_state_df(
                channel1Inputs=Channel1Inputs, channel2Inputs=Channel2Inputs
            )

            # appending metadata
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)
            dataid = datasaver.run_id

        # resetting qdac for next measurements
        self._QDict.revertPadsToDC()
        self._QDict.QDACunsync()
        self._QDict.uncoupleQdacs([value.value for value in Channel2Inputs.keys()])
        self._QDict.disableQDacFunctionGens(4)  # TODO do this for both axes's

        # Resetting QDAC labelling without custom name
        if Channel2Inputs[favorite_pad2].name is not None:
            name = Channel2Inputs[favorite_pad2].name
            if not isinstance(name, str):
                name = name.value
            self._QDict._return_channel_object(name).v.label = old_name
        return dataid

    def unpack_sweep_dict_params(
        self, ChannelInputs: Waveform1D
    ) -> tuple[list[Gate | Ohmic], list[float], list[float], list[bool], Gate | Ohmic]:
        """Unpacks common arrays from the input dictionaries for sweeps.

        Args:
            ChannelInputs (Waveform1D): Typical waveform1D like input dictionary.
                Format: {'pad' : {'amplitude' : amplitude, 'v_start' : v_start,
                                  'favorite' : bool, unit' : unit, 'name' : name}}.

        Returns:
            tuple[list[Gate | Ohmic], list[float], list[float], list[bool], Gate | Ohmic]:
                - pads (list[Gate | Ohmic]): Array of pad keys stored in the dictionary.
                - v_starts (list[float]): Array of starting voltages pulled from the dictionary.
                - amplitudes (list[float]): Array of amplitudes of waveforms pulled from the dictionary.
                - favorites (list[bool]): Array of favorites pulled from the dictionary.
                - favorite_pad (Gate | Ohmic): The favorite pad that will be stored in the database,
                  selected by the user.

        Raises:
            ValueError: If the number of favorites selected is not equal to 1.
        """
        # Extracting common arrays from the input dictionary
        pads = [pad for pad in ChannelInputs.keys()]
        v_starts = [ChannelInputs[pad].v_start for pad in ChannelInputs.keys()]
        amplitudes = [ChannelInputs[pad].amplitude for pad in ChannelInputs.keys()]
        favorites = [ChannelInputs[pad].favorite for pad in ChannelInputs.keys()]

        favorite_pad = ChannelInputs.get_favorite()

        return pads, v_starts, amplitudes, favorites, favorite_pad

    def dataframeToMetadata(self, dataframeDict, datasaver):
        """Turns dataframes stored with name keys inside of dataframeDict into qcodes metadata and saves it for reference.

        Args:
            dataframeDict (dict): Dictionary where the keys are the names of the dataframes and the values are the dataframes themselves.
            datasaver (qcodes.dataset.exporters.DataSaver): Thing generated by "with meas.run() as datasaver:".

        """
        # Iterate over the items in the dataframe dictionary
        for name, dataFrame in dataframeDict.items():
            # Initialize an empty string to store the coded dataframe
            coded_string = ""

            # Iterate over the rows of the dataframe
            for subList in dataFrame.values.tolist():
                # Join the elements of the sublist with '@@' and add it to the coded_string
                coded_string = coded_string + "@@".join(str(e) for e in subList) + "//"

            # Join the column names of the dataframe with '@@' and add it to the coded_string
            df_columns = "@@".join(dataFrame.columns.values.tolist())
            # Create the final coded dataframe by concatenating the column names and the coded string
            df_coded = df_columns + "//" + coded_string

            # Add the metadata to the dataset
            datasaver.dataset.add_metadata(tag=name, metadata=df_coded)

    def create_state_df(
        self,
        channel1Inputs: Waveform1D | None = None,
        channel2Inputs: Waveform1D | None = None,
    ) -> pd.DataFrame:
        """Function to generate a dataframe of metadata for all the gate voltages and sweeping information for each sweep.

        Args:
            channel1Inputs (dict, optional): Dictionary containing the inputs for the first channel of the sweep.
                The format is {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}.
                The fast sweep in a 2D sweep is typically the first channel, but can be changed.
            channel2Inputs (dict, optional): Dictionary containing the inputs for the second channel of the sweep.
                The format is {'pad' : {'amplitude' : amplitude, 'v_start' : v_start, 'favorite' : bool, unit' : unit, 'name' : name}}.
                The slow sweep in a 2D sweep is typically the second channel, and don't change this.

        Returns:
            pandas.DataFrame: DataFrame containing the metadata for all the gate voltages and sweeping information for each sweep.
        """
        # Extract the end voltages for all the global gates and ohms
        out: dict[Gate | Ohmic, float] = {}
        for pad in self._global_gates + self._global_ohmics:
            out[pad] = round(
                self._QDict._return_channel_object(pad.value).v.get() * 1e3, 3
            )

        # Create a StateDF object with the necessary inputs and return the DataFrame
        return StateDF(
            global_gates=self._global_gates,
            global_ohmics=self._global_ohmics,
            channel1Inputs=channel1Inputs,
            channel2Inputs=channel2Inputs,
            end_voltages=out,
        ).df

    def collect_leakage_array(
        self,
        voltage=0.001,
        sleep: float = 2,
        useOhmics=True,
        sensitivity=1e-9,
        offset: float = 0,
        maxResistance: float = 50,
        verbose=True,
    ):
        """Collects a leakage current measurement and sends it to the leaky instrument.

        Args:
            voltage : amount to shift each gate by
            offset : voltage offset for the leakage measurement if you want to test leakage when the device is accumulated
            verbose : verbose output to terminal for debugging
            maxResistance : maximum plotted resistance in MOhms. The largest allowed is 10000
            sleep : time to sleep between setting votlage and measuring (good for rc filters)

            init_currents: aka the current offsets on each of the current channels.
            These offsets were used to do the calculations of the leakage matrix, but they can be useful in diagnosing weird errors

        Returns:
            dataid : can be used to find the data in the database
        """
        if useOhmics:
            QDACVchannels = (
                self._global_gates
                + self._global_ohmics
                + self._current_channels
                + self._no_qdac_connection
            )
        else:
            QDACVchannels = self._global_gates + self._no_qdac_connection

        QDACVchannelsGoodNames: list[str] = []
        # Removes current amplifier channel names from the channel array saved and plotted
        for name in QDACVchannels:
            if name not in self._current_channels:
                if isinstance(name, str):
                    QDACVchannelsGoodNames.append(name)
                elif isinstance(name, Channel):
                    QDACVchannelsGoodNames.append(name.name)
                else:
                    QDACVchannelsGoodNames.append(name.value)
            else:
                QDACVchannelsGoodNames.append(name.name[2:])

        # converts objects to strings to be used by the driver.
        QDACVchannelsDriver: list[str] = []
        for name in QDACVchannels:
            if isinstance(name, str):
                QDACVchannelsDriver.append(name)
            elif isinstance(name, Channel):
                QDACVchannelsDriver.append(name.name)
            else:
                QDACVchannelsDriver.append(name.value)

        meas = Measurement()
        self._leaky.channels(len(QDACVchannelsGoodNames))
        self._leaky.channelNames(QDACVchannelsGoodNames)
        meas.register_parameter(self._leaky.Resistance_matrix)
        meas.register_parameter(self._leaky.offsetCurrents)
        with meas.run() as datasaver:
            # Doing the measurement and collecting the data
            data, init_currents = self._QDict.leakage_test(
                QDACVchannels=QDACVchannelsDriver,
                voltage=voltage,
                sleep=sleep,
                maxResistance=maxResistance,
                sensitivity=sensitivity,
                offset=offset,
                verbose=verbose,
            )
            # print(f"The init currents are :{init_currents}")
            # Logging data into database in proper format
            x = np.tile(
                np.linspace(
                    start=1,
                    stop=self._leaky.channels.get(),
                    num=self._leaky.channels.get(),
                ),
                reps=(self._leaky.channels.get(), 1),
            )
            y = x.T
            datasaver.add_result(
                (self._leaky.channel_xlist_matrix, x),
                (self._leaky.channel_ylist_matrix, y),
                (self._leaky.Resistance_matrix, data),
                (self._leaky.offsetCurrents, init_currents),
            )

            # Calling create_state_df to store all gate voltage/sweep metadata
            state_df = self.create_state_df(channel1Inputs=None)

            # appending metadata
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)
            dataid = datasaver.run_id
        return dataid

    def biased_illumination(
        self,
        illumination_time: float,
        illumination_current: float = 0.01,
        complianceV: float = 9,
    ):
        """Implements biased illumination using Keithley 2400 and NiDAQ (untriggered).

        INPUTS
        gates : the list of gates involved
        voltage : list of voltages referrenced by the gates list
        illumination_time : the timing for the illumination in seconds
        illumination_current : the current setting for the illumination
        complianceV : the voltage compliance for the illumination
        """
        if len(self._NDict._nidaqs.keys()) == 1:
            nidaq = self._NDict._nidaqs[list(self._NDict._nidaqs.keys())[0]]
        else:
            raise ValueError("Improper formatting of NDict")

        # Going to reduce sample rate for this measurement
        old_sample_rate = nidaq.sample_rate.get()
        nidaq.sample_rate(1000)
        nidaq.n_points(int(np.ceil(nidaq.sample_rate.get() * illumination_time)))
        num_samples_raw = nidaq.n_points.get()

        default_nidaq_timeout = 10
        timeout = int(1.1 * illumination_time)
        if int(1.1 * illumination_time) < default_nidaq_timeout:
            timeout = default_nidaq_timeout

        meas = Measurement()
        self._NDict.yaxis_labels()
        p_dict: dict[str, dict[str, Any]] = {}  # preallocated_dict
        for current_to_be_stored in self._current_channels:
            # Preallocating measurement storage parameters
            trace = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).voltage_traces
            gain = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).gain.get()
            offset = self._NDict._return_ai_channel_object(
                current_to_be_stored.name
            ).offset.get()
            p_dict[current_to_be_stored.name] = {
                "trace": trace,
                "gain": gain,
                "offset": offset,
            }
            meas.register_parameter(trace)

        # config keithley
        # self._keith.output(0)
        self._keith._set_mode_and_sense("CURR")
        self._keith.compliancev(complianceV)  # units V
        self._keith.rangei(10 * illumination_current)  # units A
        self._keith.curr(illumination_current)

        with meas.run() as datasaver:
            # with DelayedKeyboardInterrupt():
            with nidaqmx.Task("ai_task") as ai_task:
                for ch in nidaq.return_ai_channels():
                    name = ch.name[-3:]
                    ai_task.ai_channels.add_ai_voltage_chan(
                        nidaq.NiMAXname + "/" + name,
                        max_val=ch.upVrail.get(),
                        min_val=ch.botVrail.get(),
                    )
                ai_task.timing.cfg_samp_clk_timing(
                    nidaq.sample_rate.get(),
                    sample_mode=constants.AcquisitionType.FINITE,
                    samps_per_chan=num_samples_raw,
                )
                ai_task.start()
                self._keith.output(1)
                raw_data = np.array(
                    ai_task.read(
                        number_of_samples_per_channel=num_samples_raw,
                        timeout=timeout,
                    )
                )
                ai_task.wait_until_done()
                ai_task.stop()
            self._keith.output(0)
            data: dict[str, np.ndarray] = {}
            i = 0
            for Iamp in nidaq.return_ai_channels():
                Iamp = Iamp.current_channel_name
                data[Iamp] = raw_data[i]
                i += 1
            data["time"] = np.linspace(
                0, num_samples_raw / nidaq.sample_rate.get(), num_samples_raw
            )
            for current_to_be_stored in self._current_channels:
                datasaver.add_result(
                    (nidaq.time_axis, data["time"]),
                    (
                        p_dict[current_to_be_stored.name]["trace"],
                        p_dict[current_to_be_stored.name]["gain"]
                        * (
                            data[current_to_be_stored.name]
                            + p_dict[current_to_be_stored.name]["offset"]
                        ),
                    ),
                )
            dataid = datasaver.run_id

            # appending metadata
            state_df = self.create_state_df(channel1Inputs=None)
            dataframeDict = {"index_df": self._NDict._index_df, "state_df": state_df}
            self.dataframeToMetadata(dataframeDict=dataframeDict, datasaver=datasaver)

        # Resetting the sample rate back to the normal level
        nidaq.sample_rate(old_sample_rate)
        return dataid
